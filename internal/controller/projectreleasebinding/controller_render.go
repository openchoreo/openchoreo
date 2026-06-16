// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
	projectpipeline "github.com/openchoreo/openchoreo/internal/pipeline/project"
)

const ownershipConflictMarker = "RenderedRelease exists but is not owned by this binding"

// renderAndEmit drives the project pipeline against the snapshot and writes
// the resulting RenderedRelease. Returns the live RenderedRelease on success
// (Created, Updated, or already in sync) or nil when no further evaluation
// should run (render error, ownership conflict). Transient API errors are
// returned to the caller for requeue. Also populates status.namespace.
func (r *Reconciler) renderAndEmit(
	ctx context.Context,
	binding *openchoreov1alpha1.ProjectReleaseBinding,
	release *openchoreov1alpha1.ProjectRelease,
	environment *openchoreov1alpha1.Environment,
	dataPlane *openchoreov1alpha1.DataPlane,
	project *openchoreov1alpha1.Project,
) (*openchoreov1alpha1.RenderedRelease, error) {
	logger := log.FromContext(ctx)

	metadataCtx := buildMetadataContext(binding, environment, dataPlane, project)
	binding.Status.Namespace = metadataCtx.Namespace

	input := &projectpipeline.RenderInput{
		ProjectTypeSpec:    &release.Spec.ProjectType.Spec,
		ProjectParameters:  release.Spec.Parameters,
		EnvironmentConfigs: binding.Spec.EnvironmentConfigs,
		Metadata:           metadataCtx,
	}

	output, err := r.Pipeline.Render(input)
	if err != nil {
		markSyncedFalse(binding, ReasonRenderingFailed,
			fmt.Sprintf("Failed to render manifests: %v", err))
		logger.Info("Pipeline render failed", "error", err)
		return nil, nil
	}

	if reason, msg := validateRenderedResources(output.Entries, metadataCtx.Namespace); reason != "" {
		markSyncedFalse(binding, reason, msg)
		return nil, nil
	}

	manifests, err := convertEntriesToManifests(output.Entries)
	if err != nil {
		markSyncedFalse(binding, ReasonRenderingFailed,
			fmt.Sprintf("Failed to encode rendered manifests: %v", err))
		return nil, err
	}

	rrName := makeRenderedReleaseName(binding)
	rr := &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rrName,
			Namespace: binding.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, rr, func() error {
		if rr.UID != "" {
			hasOwner, ownerErr := controllerutil.HasOwnerReference(rr.GetOwnerReferences(), binding, r.Scheme)
			if ownerErr != nil {
				return fmt.Errorf("check owner reference: %w", ownerErr)
			}
			if !hasOwner {
				return fmt.Errorf("%s: %q", ownershipConflictMarker, rrName)
			}
		}

		rr.Labels = map[string]string{
			labels.LabelKeyNamespaceName:   binding.Namespace,
			labels.LabelKeyProjectName:     binding.Spec.Owner.ProjectName,
			labels.LabelKeyEnvironmentName: binding.Spec.Environment,
		}
		rr.Spec = openchoreov1alpha1.RenderedReleaseSpec{
			Owner: openchoreov1alpha1.RenderedReleaseOwner{
				ProjectName: binding.Spec.Owner.ProjectName,
			},
			EnvironmentName: binding.Spec.Environment,
			TargetPlane:     openchoreov1alpha1.TargetPlaneDataPlane,
			Resources:       manifests,
		}
		return controllerutil.SetControllerReference(binding, rr, r.Scheme)
	})

	if err != nil {
		if strings.Contains(err.Error(), ownershipConflictMarker) {
			markSyncedFalse(binding, ReasonReleaseOwnershipConflict, err.Error())
			return nil, nil
		}
		markSyncedFalse(binding, ReasonReleaseUpdateFailed,
			fmt.Sprintf("Failed to reconcile RenderedRelease %q: %v", rrName, err))
		return nil, err
	}

	switch op {
	case controllerutil.OperationResultCreated, controllerutil.OperationResultUpdated:
		controller.MarkTrueCondition(binding, ConditionSynced, ReasonReleaseCreated,
			fmt.Sprintf("RenderedRelease %q %s with %d resource(s)", rrName, op, len(manifests)))
	case controllerutil.OperationResultNone:
		controller.MarkTrueCondition(binding, ConditionSynced, ReasonReleaseSynced,
			fmt.Sprintf("RenderedRelease %q is up to date", rrName))
	}

	return rr, nil
}

// makeRenderedReleaseName returns the RenderedRelease name for a binding.
// The "p_" discriminator on the first hash input ensures collision-safety
// against component-side and resource-side RenderedReleases: the underscore
// is non-alphanumeric, so dpkubernetes.sanitizeName replaces it with "-" in
// the visible name (giving a "p-" prefix) but the original string with the
// underscore drives the SHA hash. K8s name validation forbids "_" in
// user-supplied names, so a Resource or Component happening to be named
// "p-{project}" hashes a different input string and produces a different
// final name despite the matching visible base.
func makeRenderedReleaseName(binding *openchoreov1alpha1.ProjectReleaseBinding) string {
	return dpkubernetes.GenerateK8sName("p_"+binding.Spec.Owner.ProjectName, binding.Spec.Environment)
}

// convertEntriesToManifests marshals each pipeline-rendered map into a
// RenderedManifest with its ID preserved verbatim. The pipeline already
// supplies IDs from (Cluster)ProjectType.spec.resources[].id (with a
// -<index> suffix for forEach iterations); this controller does not
// regenerate them from kind/name.
func convertEntriesToManifests(entries []projectpipeline.RenderedEntry) ([]openchoreov1alpha1.RenderedManifest, error) {
	manifests := make([]openchoreov1alpha1.RenderedManifest, 0, len(entries))
	for i := range entries {
		raw, err := json.Marshal(entries[i].Object)
		if err != nil {
			return nil, fmt.Errorf("marshal rendered manifest %q: %w", entries[i].ID, err)
		}
		manifests = append(manifests, openchoreov1alpha1.RenderedManifest{
			ID:     entries[i].ID,
			Object: &runtime.RawExtension{Raw: raw},
		})
	}
	return manifests, nil
}

// buildMetadataContext computes the platform-injected metadata surface
// exposed to CEL templates. Namespace follows the platform naming scheme
// dp-{ns}-{project}-{env}-{hash}; the remaining fields are populated from
// the binding, environment, dataplane, and owning project.
func buildMetadataContext(
	binding *openchoreov1alpha1.ProjectReleaseBinding,
	environment *openchoreov1alpha1.Environment,
	dataPlane *openchoreov1alpha1.DataPlane,
	project *openchoreov1alpha1.Project,
) projectpipeline.MetadataContext {
	projectName := binding.Spec.Owner.ProjectName
	envName := binding.Spec.Environment
	bindingNamespace := binding.Namespace

	namespace := dpkubernetes.GenerateK8sNameWithLengthLimit(
		dpkubernetes.MaxNamespaceNameLength,
		"dp", bindingNamespace, projectName, envName,
	)

	standardLabels := map[string]string{
		labels.LabelKeyNamespaceName:   bindingNamespace,
		labels.LabelKeyProjectName:     projectName,
		labels.LabelKeyProjectUID:      string(project.UID),
		labels.LabelKeyEnvironmentName: envName,
		labels.LabelKeyEnvironmentUID:  string(environment.UID),
	}

	return projectpipeline.MetadataContext{
		Namespace:        namespace,
		ProjectNamespace: bindingNamespace,
		ProjectName:      projectName,
		ProjectUID:       string(project.UID),
		EnvironmentName:  envName,
		EnvironmentUID:   string(environment.UID),
		DataPlaneName:    dataPlane.Name,
		DataPlaneUID:     string(dataPlane.UID),
		Labels:           standardLabels,
		Annotations:      binding.Annotations,
	}
}

// fetchSupportingResources resolves the Environment, DataPlane, and Project
// the binding references. On NotFound it marks Synced=False with the
// appropriate reason and returns (nil, nil, nil, nil); transient errors are
// returned for requeue.
func (r *Reconciler) fetchSupportingResources(
	ctx context.Context,
	binding *openchoreov1alpha1.ProjectReleaseBinding,
) (*openchoreov1alpha1.Environment, *openchoreov1alpha1.DataPlane, *openchoreov1alpha1.Project, error) {
	logger := log.FromContext(ctx)

	environment := &openchoreov1alpha1.Environment{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      binding.Spec.Environment,
		Namespace: binding.Namespace,
	}, environment); err != nil {
		if apierrors.IsNotFound(err) {
			markSyncedFalse(binding, ReasonEnvironmentNotFound,
				fmt.Sprintf("Environment %q not found", binding.Spec.Environment))
			return nil, nil, nil, nil
		}
		logger.Error(err, "Failed to get Environment", "environment", binding.Spec.Environment)
		return nil, nil, nil, err
	}

	dataPlaneResult, err := controller.GetDataPlaneFromRef(ctx, r.Client, environment.Namespace, environment.Spec.DataPlaneRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			markSyncedFalse(binding, ReasonDataPlaneNotFound,
				fmt.Sprintf("DataPlane not found for environment %q", environment.Name))
			return nil, nil, nil, nil
		}
		logger.Error(err, "Failed to resolve DataPlane", "environment", environment.Name)
		return nil, nil, nil, err
	}
	dataPlane := dataPlaneResult.ToDataPlane()

	project := &openchoreov1alpha1.Project{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      binding.Spec.Owner.ProjectName,
		Namespace: binding.Namespace,
	}, project); err != nil {
		if apierrors.IsNotFound(err) {
			markSyncedFalse(binding, ReasonProjectNotFound,
				fmt.Sprintf("Project %q not found", binding.Spec.Owner.ProjectName))
			return nil, nil, nil, nil
		}
		logger.Error(err, "Failed to get Project", "project", binding.Spec.Owner.ProjectName)
		return nil, nil, nil, err
	}

	return environment, dataPlane, project, nil
}
