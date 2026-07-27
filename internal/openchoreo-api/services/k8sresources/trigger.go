// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

const (
	workloadTypeCronJob = "cronjob"
	// maxJobNameLength is the maximum length of a Kubernetes resource name.
	maxJobNameLength = 63
	// jobNameRandomSuffixLength is the length of the random suffix appended to triggered Job names
	// to avoid collisions between triggers within the same second.
	jobNameRandomSuffixLength = 5
	// instantiateAnnotationKey mirrors the annotation kubectl adds when creating a Job from a CronJob.
	instantiateAnnotationKey   = "cronjob.kubernetes.io/instantiate"
	instantiateAnnotationValue = "manual"
)

// TriggerCronJob creates a Job from the deployed CronJob's spec.jobTemplate with an owner
// reference back to the CronJob, matching `kubectl create job --from=cronjob/<name>`.
// It is only allowed when the release binding's component is a cronjob workload.
func (s *k8sResourcesService) TriggerCronJob(ctx context.Context, namespaceName, releaseBindingName string) (*models.CronJobTriggerResponse, error) {
	s.logger.Debug("Triggering cronjob", "namespace", namespaceName, "releaseBinding", releaseBindingName)

	if s.gatewayClient == nil {
		return nil, fmt.Errorf("gateway client is not configured")
	}

	// Verify the component is a cronjob workload using the frozen ComponentRelease snapshot.
	if err := s.assertCronJobWorkload(ctx, namespaceName, releaseBindingName); err != nil {
		return nil, err
	}

	// Resolve the data plane release contexts owned by the binding.
	releaseContexts, err := s.resolveReleaseContexts(ctx, namespaceName, releaseBindingName)
	if err != nil {
		return nil, err
	}

	rc, cronJobStatus := findCronJobResource(releaseContexts)
	if rc == nil {
		return nil, ErrCronJobNotFound
	}

	// Fetch the live CronJob to read its jobTemplate and uid.
	plural, err := s.resolveResourcePlural(cronJobStatus.Group, cronJobStatus.Version, cronJobStatus.Kind)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cronjob plural: %w", err)
	}
	getPath := buildK8sGetPath(cronJobStatus.Group, cronJobStatus.Version, plural, cronJobStatus.Namespace, cronJobStatus.Name)
	cronJob, err := s.fetchLiveResource(ctx, rc.plane, getPath)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cronjob: %w", err)
	}

	job, err := buildJobFromCronJob(cronJob)
	if err != nil {
		return nil, err
	}

	if err := s.createJob(ctx, rc.plane, cronJobStatus.Namespace, job); err != nil {
		return nil, err
	}

	return &models.CronJobTriggerResponse{
		JobName:     getNestedString(job, "metadata", "name"),
		Namespace:   cronJobStatus.Namespace,
		CronJobName: cronJobStatus.Name,
	}, nil
}

// assertCronJobWorkload returns ErrNotCronJobWorkload unless the release binding's ComponentRelease
// snapshot has workloadType cronjob.
func (s *k8sResourcesService) assertCronJobWorkload(ctx context.Context, namespaceName, releaseBindingName string) error {
	var rb openchoreov1alpha1.ReleaseBinding
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: releaseBindingName}, &rb); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrReleaseBindingNotFound
		}
		return fmt.Errorf("failed to get release binding: %w", err)
	}

	// No ComponentRelease is bound yet (e.g. nothing deployed) — this is a "not found"
	// condition, not a workload-type mismatch.
	if rb.Spec.ReleaseName == "" {
		return ErrComponentReleaseNotFound
	}

	var release openchoreov1alpha1.ComponentRelease
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: rb.Spec.ReleaseName}, &release); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrComponentReleaseNotFound
		}
		return fmt.Errorf("failed to get component release: %w", err)
	}

	if !strings.EqualFold(release.Spec.ComponentType.Spec.WorkloadType, workloadTypeCronJob) {
		return ErrNotCronJobWorkload
	}
	return nil
}

// findCronJobResource returns the release context and status entry for the first CronJob found on a
// data plane release owned by the binding.
func findCronJobResource(contexts []releaseContext) (*releaseContext, *openchoreov1alpha1.RenderedManifestStatus) {
	for i := range contexts {
		rc := &contexts[i]
		if rc.plane.planeType != planeTypeDataPlane {
			continue
		}
		for j := range rc.release.Status.Resources {
			rs := &rc.release.Status.Resources[j]
			if rs.Kind == "CronJob" && rs.Group == "batch" {
				return rc, rs
			}
		}
	}
	return nil, nil
}

// buildJobFromCronJob builds a Job manifest from a CronJob's spec.jobTemplate, adding an owner
// reference to the CronJob and the manual-instantiate annotation, mirroring kubectl behavior.
func buildJobFromCronJob(cronJob map[string]any) (map[string]any, error) {
	cronJobName := getNestedString(cronJob, "metadata", "name")
	cronJobNamespace := getNestedString(cronJob, "metadata", "namespace")
	cronJobUID := getNestedString(cronJob, "metadata", "uid")
	if cronJobName == "" || cronJobUID == "" {
		return nil, fmt.Errorf("cronjob is missing name or uid")
	}

	spec, ok := cronJob["spec"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("cronjob has no spec")
	}
	jobTemplate, ok := spec["jobTemplate"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("cronjob has no spec.jobTemplate")
	}
	jobSpec, ok := jobTemplate["spec"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("cronjob has no spec.jobTemplate.spec")
	}

	metadata := map[string]any{
		"name":      makeJobName(cronJobName),
		"namespace": cronJobNamespace,
		"annotations": map[string]any{
			instantiateAnnotationKey: instantiateAnnotationValue,
		},
		"ownerReferences": []any{
			map[string]any{
				"apiVersion":         "batch/v1",
				"kind":               "CronJob",
				"name":               cronJobName,
				"uid":                cronJobUID,
				"controller":         true,
				"blockOwnerDeletion": true,
			},
		},
	}

	// Carry over labels and annotations defined on the CronJob's jobTemplate metadata.
	if tmplMeta, ok := jobTemplate["metadata"].(map[string]any); ok {
		if labels, ok := tmplMeta["labels"].(map[string]any); ok && len(labels) > 0 {
			metadata["labels"] = labels
		}
		if anns, ok := tmplMeta["annotations"].(map[string]any); ok {
			merged := map[string]any{instantiateAnnotationKey: instantiateAnnotationValue}
			for k, v := range anns {
				merged[k] = v
			}
			metadata["annotations"] = merged
		}
	}

	return map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata":   metadata,
		"spec":       jobSpec,
	}, nil
}

// makeJobName builds a unique Job name from the CronJob name, the current unix timestamp, and a
// short random suffix. The random suffix avoids collisions when the same CronJob is triggered more
// than once within the same second (e.g. a double-click or client retry), which a timestamp alone
// cannot guarantee. The prefix is truncated so the full name stays within the 63-character limit.
func makeJobName(cronJobName string) string {
	suffix := "-" + strconv.FormatInt(time.Now().Unix(), 10) + "-" + rand.String(jobNameRandomSuffixLength)
	maxPrefix := maxJobNameLength - len(suffix)
	if len(cronJobName) > maxPrefix {
		cronJobName = cronJobName[:maxPrefix]
	}
	return cronJobName + suffix
}

// createJob POSTs the Job manifest to the data plane through the gateway.
func (s *k8sResourcesService) createJob(ctx context.Context, pi planeInfo, namespace string, job map[string]any) error {
	body, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	k8sPath := buildK8sListPath("batch", "v1", "jobs", namespace)
	resp, err := s.gatewayClient.PostK8sRequest(ctx, pi.planeType, pi.planeID, pi.crNamespace, pi.crName, k8sPath, body)
	if err != nil {
		return fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		return nil
	}

	// A name collision (two triggers landing on the same generated name) is a retryable
	// condition, not an internal failure, so surface it distinctly.
	if resp.StatusCode == http.StatusConflict {
		return ErrTriggerConflict
	}

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	return fmt.Errorf("failed to create job, status %d: %s", resp.StatusCode, string(respBody))
}
