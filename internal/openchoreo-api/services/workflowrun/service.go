// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	ocLabels "github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// workflowRunService handles workflow run business logic without authorization checks.
type workflowRunService struct {
	k8sClient           client.Client
	planeClientProvider kubernetesClient.WorkflowPlaneClientProvider
	gwClient            *gatewayClient.Client
	logger              *slog.Logger
}

var _ Service = (*workflowRunService)(nil)

var workflowRunTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "WorkflowRun",
}

// NewService creates a new workflow run service without authorization.
func NewService(k8sClient client.Client, planeClientProvider kubernetesClient.WorkflowPlaneClientProvider, gwClient *gatewayClient.Client, logger *slog.Logger) Service {
	return &workflowRunService{
		k8sClient:           k8sClient,
		planeClientProvider: planeClientProvider,
		gwClient:            gwClient,
		logger:              logger,
	}
}

func (s *workflowRunService) CreateWorkflowRun(ctx context.Context, namespaceName string, wfRun *openchoreov1alpha1.WorkflowRun) (*openchoreov1alpha1.WorkflowRun, error) {
	if wfRun == nil {
		return nil, fmt.Errorf("workflow run cannot be nil")
	}

	s.logger.Debug("Creating workflow run", "namespace", namespaceName, "name", wfRun.Name)

	// Verify the referenced workflow exists based on the kind
	switch wfRun.Spec.Workflow.Kind {
	case openchoreov1alpha1.WorkflowRefKindClusterWorkflow:
		clusterWorkflow := &openchoreov1alpha1.ClusterWorkflow{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{
			Name: wfRun.Spec.Workflow.Name,
		}, clusterWorkflow); err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Warn("Referenced cluster workflow not found", "workflow", wfRun.Spec.Workflow.Name)
				return nil, ErrWorkflowNotFound
			}
			s.logger.Error("Failed to get referenced cluster workflow", "error", err)
			return nil, fmt.Errorf("failed to get referenced cluster workflow: %w", err)
		}
	default:
		workflow := &openchoreov1alpha1.Workflow{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{
			Name:      wfRun.Spec.Workflow.Name,
			Namespace: namespaceName,
		}, workflow); err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Warn("Referenced workflow not found", "namespace", namespaceName, "workflow", wfRun.Spec.Workflow.Name)
				return nil, ErrWorkflowNotFound
			}
			s.logger.Error("Failed to get referenced workflow", "error", err)
			return nil, fmt.Errorf("failed to get referenced workflow: %w", err)
		}
	}

	// Ensure namespace is set
	wfRun.Namespace = namespaceName
	wfRun.Status = openchoreov1alpha1.WorkflowRunStatus{}

	if err := s.k8sClient.Create(ctx, wfRun); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Workflow run already exists", "namespace", namespaceName, "name", wfRun.Name)
			return nil, ErrWorkflowRunAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create workflow run", "error", err)
		return nil, fmt.Errorf("failed to create workflow run: %w", err)
	}

	wfRun.TypeMeta = workflowRunTypeMeta
	s.logger.Debug("Workflow run created successfully", "namespace", namespaceName, "name", wfRun.Name)
	return wfRun, nil
}

func (s *workflowRunService) UpdateWorkflowRun(ctx context.Context, namespaceName string, wfRun *openchoreov1alpha1.WorkflowRun) (*openchoreov1alpha1.WorkflowRun, error) {
	if wfRun == nil {
		return nil, fmt.Errorf("workflow run cannot be nil")
	}

	s.logger.Debug("Updating workflow run", "namespace", namespaceName, "name", wfRun.Name)

	// Retry on conflict because the controller constantly updates the status subresource,
	// which bumps resourceVersion and can cause optimistic concurrency failures.
	var existing *openchoreov1alpha1.WorkflowRun
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing = &openchoreov1alpha1.WorkflowRun{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{
			Name:      wfRun.Name,
			Namespace: namespaceName,
		}, existing); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return ErrWorkflowRunNotFound
			}
			return fmt.Errorf("failed to get workflow run: %w", err)
		}

		// Only apply user-mutable fields to the existing object, preserving server-managed fields
		existing.Labels = wfRun.Labels
		existing.Annotations = wfRun.Annotations
		existing.Spec = wfRun.Spec

		return s.k8sClient.Update(ctx, existing)
	})
	if err != nil {
		if errors.Is(err, ErrWorkflowRunNotFound) {
			s.logger.Warn("Workflow run not found", "namespace", namespaceName, "name", wfRun.Name)
			return nil, ErrWorkflowRunNotFound
		}
		if apierrors.IsInvalid(err) {
			s.logger.Error("Workflow run update rejected by validation", "error", err)
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update workflow run", "error", err)
		return nil, fmt.Errorf("failed to update workflow run: %w", err)
	}

	existing.TypeMeta = workflowRunTypeMeta
	s.logger.Debug("Workflow run updated successfully", "namespace", namespaceName, "name", wfRun.Name)
	return existing, nil
}

func (s *workflowRunService) ListWorkflowRuns(ctx context.Context, namespaceName, projectName, componentName, workflowName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.WorkflowRun], error) {
	s.logger.Debug("Listing workflow runs", "namespace", namespaceName, "project", projectName, "component", componentName, "workflow", workflowName, "limit", opts.Limit, "cursor", opts.Cursor)

	listResource := s.listWorkflowRunsResource(namespaceName)

	// Apply label filters if project or component specified
	var filters []services.ItemFilter[openchoreov1alpha1.WorkflowRun]
	if projectName != "" {
		filters = append(filters, func(wr openchoreov1alpha1.WorkflowRun) bool {
			return wr.Labels[ocLabels.LabelKeyProjectName] == projectName
		})
	}
	if componentName != "" {
		filters = append(filters, func(wr openchoreov1alpha1.WorkflowRun) bool {
			return wr.Labels[ocLabels.LabelKeyComponentName] == componentName
		})
	}
	if workflowName != "" {
		filters = append(filters, func(wr openchoreov1alpha1.WorkflowRun) bool {
			return wr.Spec.Workflow.Name == workflowName
		})
	}

	return services.PreFilteredList(listResource, filters...)(ctx, opts)
}

// listWorkflowRunsResource returns a ListResource that fetches workflow runs from K8s for the given namespace.
func (s *workflowRunService) listWorkflowRunsResource(namespaceName string) services.ListResource[openchoreov1alpha1.WorkflowRun] {
	return func(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.WorkflowRun], error) {
		commonOpts, err := services.BuildListOptions(opts)
		if err != nil {
			return nil, err
		}
		listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

		var wfRunList openchoreov1alpha1.WorkflowRunList
		if err := s.k8sClient.List(ctx, &wfRunList, listOpts...); err != nil {
			s.logger.Error("Failed to list workflow runs", "error", err)
			return nil, fmt.Errorf("failed to list workflow runs: %w", err)
		}

		for i := range wfRunList.Items {
			wfRunList.Items[i].TypeMeta = workflowRunTypeMeta
		}

		result := &services.ListResult[openchoreov1alpha1.WorkflowRun]{
			Items:      wfRunList.Items,
			NextCursor: wfRunList.Continue,
		}
		if wfRunList.RemainingItemCount != nil {
			remaining := *wfRunList.RemainingItemCount
			result.RemainingCount = &remaining
		}

		return result, nil
	}
}

func (s *workflowRunService) GetWorkflowRun(ctx context.Context, namespaceName, runName string) (*openchoreov1alpha1.WorkflowRun, error) {
	s.logger.Debug("Getting workflow run", "namespace", namespaceName, "run", runName)

	wfRun := &openchoreov1alpha1.WorkflowRun{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      runName,
		Namespace: namespaceName,
	}, wfRun); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workflow run not found", "namespace", namespaceName, "run", runName)
			return nil, ErrWorkflowRunNotFound
		}
		s.logger.Error("Failed to get workflow run", "error", err)
		return nil, fmt.Errorf("failed to get workflow run: %w", err)
	}

	wfRun.TypeMeta = workflowRunTypeMeta
	return wfRun, nil
}

func (s *workflowRunService) DeleteWorkflowRun(ctx context.Context, namespaceName, runName string) error {
	s.logger.Debug("Deleting workflow run", "namespace", namespaceName, "run", runName)

	wfRun := &openchoreov1alpha1.WorkflowRun{}
	wfRun.Name = runName
	wfRun.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, wfRun); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrWorkflowRunNotFound
		}
		s.logger.Error("Failed to delete workflow run", "error", err)
		return fmt.Errorf("failed to delete workflow run: %w", err)
	}

	s.logger.Debug("Workflow run deleted successfully", "namespace", namespaceName, "run", runName)
	return nil
}

// GetWorkflowRunLogs retrieves logs from a workflow run.
func (s *workflowRunService) GetWorkflowRunLogs(ctx context.Context, namespaceName, runName, taskName string, sinceSeconds *int64) ([]models.WorkflowRunLogEntry, error) {
	logger := s.logger.With("namespace", namespaceName, "run", runName, "task", taskName, "sinceSeconds", sinceSeconds)
	logger.Debug("Getting workflow run logs")

	// Get WorkflowRun
	var workflowRun openchoreov1alpha1.WorkflowRun
	if err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      runName,
		Namespace: namespaceName,
	}, &workflowRun); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Warn("Workflow run not found", "namespace", namespaceName, "run", runName)
			return nil, ErrWorkflowRunNotFound
		}
		logger.Error("Failed to get workflow run", "error", err)
		return nil, fmt.Errorf("failed to get workflow run: %w", err)
	}

	// Check if RunReference exists
	if workflowRun.Status.RunReference == nil || workflowRun.Status.RunReference.Name == "" || workflowRun.Status.RunReference.Namespace == "" {
		logger.Error("Workflow run reference not found", "run", runName)
		return nil, fmt.Errorf("%w: %s", ErrWorkflowRunReferenceNotFound, runName)
	}

	// Resolve the workflow's workflowPlaneRef
	workflowPlaneRef, err := s.resolveWorkflowPlaneRef(ctx, namespaceName, workflowRun.Spec.Workflow)
	if err != nil {
		logger.Error("Failed to resolve workflow plane ref", "error", err)
		return nil, fmt.Errorf("failed to resolve workflow plane ref: %w", err)
	}

	return s.getArgoWorkflowRunLogs(ctx, namespaceName, workflowRun.Status.RunReference, workflowPlaneRef, taskName, sinceSeconds)
}

// getArgoWorkflowRunLogs retrieves logs from an Argo Workflow run.
func (s *workflowRunService) getArgoWorkflowRunLogs(
	ctx context.Context,
	namespaceName string,
	runReference *openchoreov1alpha1.ResourceReference,
	workflowPlaneRef *openchoreov1alpha1.WorkflowPlaneRef,
	taskName string,
	sinceSeconds *int64,
) ([]models.WorkflowRunLogEntry, error) {
	logger := s.logger.With("namespace", namespaceName, "runReference", runReference, "task", taskName, "sinceSeconds", sinceSeconds)
	logger.Debug("Getting Argo workflow run logs")

	// Get workflow plane client
	wpClient, err := s.getWorkflowPlaneClient(ctx, namespaceName, workflowPlaneRef)
	if err != nil {
		logger.Error("Failed to get workflow plane client", "error", err)
		return nil, fmt.Errorf("failed to get workflow plane client: %w", err)
	}

	// Get Argo Workflow from workflow plane
	var argoWorkflow argoproj.Workflow
	if err := wpClient.Get(ctx, types.NamespacedName{
		Name:      runReference.Name,
		Namespace: runReference.Namespace,
	}, &argoWorkflow); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Warn("Argo workflow not found in workflow plane", "workflow", runReference.Name, "namespace", runReference.Namespace, "error", err)
			return nil, fmt.Errorf("argo workflow not found: %w", err)
		}
		logger.Error("Failed to get argo workflow", "error", err)
		return nil, fmt.Errorf("failed to get argo workflow: %w", err)
	}

	// Get pods for the workflow/task
	pods, err := s.getArgoWorkflowPodsForLogs(ctx, wpClient, &argoWorkflow, taskName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow pods: %w", err)
	}

	// Get workflow plane resource
	workflowPlane, err := s.resolveWorkflowPlane(ctx, namespaceName, workflowPlaneRef)
	if err != nil {
		logger.Error("Failed to get workflow plane", "error", err)
		return nil, fmt.Errorf("failed to get workflow plane: %w", err)
	}

	// Get logs from pods and convert to structured format
	allLogEntries := make([]models.WorkflowRunLogEntry, 0)
	for _, pod := range pods {
		podLogs, err := s.getArgoWorkflowPodLogs(ctx, workflowPlane, &pod, sinceSeconds)
		if err != nil {
			logger.Warn("Failed to get logs from pod", "pod", pod.Name, "error", err)
			return nil, fmt.Errorf("failed to get logs from pod: %w", err)
		}

		// Parse log string into individual lines and create log entries
		lines := strings.Split(podLogs, "\n")
		for _, line := range lines {
			trimmedLine := strings.TrimSpace(line)
			if trimmedLine == "" || trimmedLine == "---" {
				continue
			}

			// Extract timestamp and log message
			spaceIndex := strings.Index(trimmedLine, " ")
			if spaceIndex > 0 {
				timestampCandidate := trimmedLine[:spaceIndex]
				if _, err := time.Parse(time.RFC3339, timestampCandidate); err == nil {
					allLogEntries = append(allLogEntries, models.WorkflowRunLogEntry{
						Timestamp: timestampCandidate,
						Log:       trimmedLine[spaceIndex+1:],
					})
					continue
				}
				if _, err := time.Parse(time.RFC3339Nano, timestampCandidate); err == nil {
					allLogEntries = append(allLogEntries, models.WorkflowRunLogEntry{
						Timestamp: timestampCandidate,
						Log:       trimmedLine[spaceIndex+1:],
					})
					continue
				}
			}

			allLogEntries = append(allLogEntries, models.WorkflowRunLogEntry{
				Timestamp: "",
				Log:       trimmedLine,
			})
		}
	}

	return allLogEntries, nil
}

// resolveWorkflowPlane resolves the workflow plane using the workflow's workflowPlaneRef.
func (s *workflowRunService) resolveWorkflowPlane(ctx context.Context, namespaceName string, ref *openchoreov1alpha1.WorkflowPlaneRef) (*openchoreov1alpha1.WorkflowPlane, error) {
	workflowPlaneResult, err := controller.GetWorkflowPlaneFromRef(ctx, s.k8sClient, namespaceName, ref)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve workflow plane: %w", err)
	}
	if workflowPlaneResult == nil {
		return nil, fmt.Errorf("no workflow plane found for namespace: %s", namespaceName)
	}
	// Convert WorkflowPlaneResult to WorkflowPlane for downstream gateway client compatibility
	if workflowPlaneResult.WorkflowPlane != nil {
		return workflowPlaneResult.WorkflowPlane, nil
	}
	if workflowPlaneResult.ClusterWorkflowPlane != nil {
		// Build a facade WorkflowPlane from ClusterWorkflowPlane for the gateway client.
		// Namespace must be "_cluster" for cluster-scoped planes so the gateway proxy
		// constructs the correct URL path segment.
		return &openchoreov1alpha1.WorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workflowPlaneResult.ClusterWorkflowPlane.Name,
				Namespace: "_cluster",
			},
			Spec: openchoreov1alpha1.WorkflowPlaneSpec{
				PlaneID: workflowPlaneResult.ClusterWorkflowPlane.Spec.PlaneID,
			},
		}, nil
	}
	return nil, fmt.Errorf("no workflow plane found for namespace: %s", namespaceName)
}

// resolveWorkflowPlaneRef resolves the WorkflowPlaneRef for a given WorkflowRun by looking up its workflow.
func (s *workflowRunService) resolveWorkflowPlaneRef(ctx context.Context, namespaceName string, workflowRef openchoreov1alpha1.WorkflowRunConfig) (*openchoreov1alpha1.WorkflowPlaneRef, error) {
	workflowResult, err := controller.ResolveWorkflow(ctx, s.k8sClient, namespaceName, workflowRef.Kind, workflowRef.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve workflow '%s' (kind: %s): %w", workflowRef.Name, workflowRef.Kind, err)
	}
	return workflowResult.GetWorkflowSpec().WorkflowPlaneRef, nil
}

// getWorkflowPlaneClient creates and returns a Kubernetes client for the workflow plane cluster.
func (s *workflowRunService) getWorkflowPlaneClient(ctx context.Context, namespaceName string, ref *openchoreov1alpha1.WorkflowPlaneRef) (client.Client, error) {
	s.logger.Debug("Getting workflow plane client", "namespace", namespaceName)

	workflowPlaneResult, err := controller.GetWorkflowPlaneFromRef(ctx, s.k8sClient, namespaceName, ref)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve workflow plane: %w", err)
	}
	if workflowPlaneResult == nil {
		return nil, fmt.Errorf("no workflow plane found for namespace: %s", namespaceName)
	}

	workflowPlaneClient, err := workflowPlaneResult.GetK8sClient(s.planeClientProvider)
	if err != nil {
		s.logger.Error("Failed to create workflow plane client", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to create workflow plane client: %w", err)
	}

	s.logger.Debug("Created workflow plane client", "namespace", namespaceName, "plane", workflowPlaneResult.GetName())
	return workflowPlaneClient, nil
}

// listAndFilterWorkflowPods lists all pods for a workflow and filters them by taskName.
// When allowSubstring is true, pods whose node-name annotation is absent are also matched
// by checking whether the pod name contains taskName (used for log retrieval).
func (s *workflowRunService) listAndFilterWorkflowPods(ctx context.Context, wpClient client.Client, workflow *argoproj.Workflow, taskName string, allowSubstring bool) ([]corev1.Pod, error) {
	selector := labels.Set{
		"workflows.argoproj.io/workflow": workflow.Name,
	}

	var podList corev1.PodList
	if err := wpClient.List(ctx, &podList, client.InNamespace(workflow.Namespace), client.MatchingLabels(selector)); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	if len(podList.Items) == 0 {
		return []corev1.Pod{}, nil
	}

	if taskName != "" {
		filteredPods := make([]corev1.Pod, 0)
		for _, pod := range podList.Items {
			nodeName := pod.Annotations["workflows.argoproj.io/node-name"]
			if matchesTaskName(nodeName, taskName) ||
				(allowSubstring && nodeName == "" && strings.Contains(pod.Name, taskName)) {
				filteredPods = append(filteredPods, pod)
			}
		}
		if len(filteredPods) == 0 {
			return []corev1.Pod{}, nil
		}
		return filteredPods, nil
	}

	return podList.Items, nil
}

// matchesTaskName checks if an Argo node-name annotation matches a task name.
// Argo uses the format "<workflow>[<index>].<task-name>" for the node-name annotation,
// e.g. "greeting-service-build-01[0].checkout-source" for task "checkout-source".
func matchesTaskName(nodeName, taskName string) bool {
	if nodeName == taskName {
		return true
	}
	// Check if the node name ends with ".<taskName>" (after the [index] part)
	dotIdx := strings.LastIndex(nodeName, ".")
	if dotIdx >= 0 && nodeName[dotIdx+1:] == taskName {
		return true
	}
	return false
}

// getArgoWorkflowPodsForLogs finds pods for a workflow and optionally a specific task (for log retrieval).
// Falls back to a pod-name substring match when the node-name annotation is absent.
func (s *workflowRunService) getArgoWorkflowPodsForLogs(ctx context.Context, wpClient client.Client, workflow *argoproj.Workflow, taskName string) ([]corev1.Pod, error) {
	return s.listAndFilterWorkflowPods(ctx, wpClient, workflow, taskName, true)
}

// getArgoWorkflowPodLogs retrieves logs from an Argo Workflow pod using the gateway client.
func (s *workflowRunService) getArgoWorkflowPodLogs(ctx context.Context, workflowPlane *openchoreov1alpha1.WorkflowPlane, pod *corev1.Pod, sinceSeconds *int64) (string, error) {
	if s.gwClient == nil {
		return "", fmt.Errorf("gateway client is not configured")
	}

	excludedContainersForLogs := map[string]bool{
		"wait": true,
		"init": true,
	}

	containerNames := make([]string, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		if !excludedContainersForLogs[container.Name] {
			containerNames = append(containerNames, container.Name)
		}
	}
	if len(containerNames) == 0 {
		return "", fmt.Errorf("no containers to fetch logs from in pod")
	}

	var allLogs strings.Builder
	for i, containerName := range containerNames {
		if i > 0 {
			allLogs.WriteString("\n---\n")
		}

		logs, err := s.gwClient.GetPodLogsFromPlane(ctx, "workflowplane", workflowPlane.Spec.PlaneID, workflowPlane.Namespace, workflowPlane.Name,
			&gatewayClient.PodReference{
				Namespace: pod.Namespace,
				Name:      pod.Name,
			}, &gatewayClient.PodLogsOptions{
				ContainerName:     containerName,
				IncludeTimestamps: true,
				SinceSeconds:      sinceSeconds,
			})
		if err != nil {
			s.logger.Warn("Failed to fetch logs from container", "pod", pod.Name, "container", containerName, "error", err)
			continue
		}

		allLogs.WriteString(logs)
	}

	return allLogs.String(), nil
}

// GetWorkflowRunEvents retrieves events from a workflow run.
func (s *workflowRunService) GetWorkflowRunEvents(ctx context.Context, namespaceName, runName, taskName string) ([]models.WorkflowRunEventEntry, error) {
	logger := s.logger.With("namespace", namespaceName, "run", runName, "task", taskName)
	logger.Debug("Getting workflow run events")

	// Get WorkflowRun
	var workflowRun openchoreov1alpha1.WorkflowRun
	if err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      runName,
		Namespace: namespaceName,
	}, &workflowRun); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Warn("Workflow run not found", "namespace", namespaceName, "run", runName)
			return nil, ErrWorkflowRunNotFound
		}
		logger.Error("Failed to get workflow run", "error", err)
		return nil, fmt.Errorf("failed to get workflow run: %w", err)
	}

	// Check if RunReference exists
	if workflowRun.Status.RunReference == nil || workflowRun.Status.RunReference.Name == "" || workflowRun.Status.RunReference.Namespace == "" {
		logger.Warn("Workflow run reference not found", "run", runName)
		return nil, fmt.Errorf("%w: %s", ErrWorkflowRunReferenceNotFound, runName)
	}

	// Resolve the workflow's workflowPlaneRef
	workflowPlaneRef, err := s.resolveWorkflowPlaneRef(ctx, namespaceName, workflowRun.Spec.Workflow)
	if err != nil {
		logger.Error("Failed to resolve workflow plane ref", "error", err)
		return nil, fmt.Errorf("failed to resolve workflow plane ref: %w", err)
	}

	return s.getArgoWorkflowRunEvents(ctx, namespaceName, workflowRun.Status.RunReference, workflowPlaneRef, taskName)
}

// getArgoWorkflowRunEvents retrieves events from an Argo Workflow run.
func (s *workflowRunService) getArgoWorkflowRunEvents(
	ctx context.Context,
	namespaceName string,
	runReference *openchoreov1alpha1.ResourceReference,
	workflowPlaneRef *openchoreov1alpha1.WorkflowPlaneRef,
	taskName string,
) ([]models.WorkflowRunEventEntry, error) {
	logger := s.logger.With("namespace", namespaceName, "runReference", runReference, "task", taskName)
	logger.Debug("Getting Argo workflow run events")

	// Get workflow plane client
	wpClient, err := s.getWorkflowPlaneClient(ctx, namespaceName, workflowPlaneRef)
	if err != nil {
		logger.Error("Failed to get workflow plane client", "error", err)
		return nil, fmt.Errorf("failed to get workflow plane client: %w", err)
	}

	// Get Argo Workflow from workflow plane
	var argoWorkflow argoproj.Workflow
	if err := wpClient.Get(ctx, types.NamespacedName{
		Name:      runReference.Name,
		Namespace: runReference.Namespace,
	}, &argoWorkflow); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Warn("Argo workflow not found in workflow plane", "workflow", runReference.Name, "namespace", runReference.Namespace)
			return nil, fmt.Errorf("argo workflow not found: %w", err)
		}
		logger.Error("Failed to get argo workflow", "error", err)
		return nil, fmt.Errorf("failed to get argo workflow: %w", err)
	}

	// Get pods for the workflow/task (strict matching for events)
	pods, err := s.getArgoWorkflowPods(ctx, wpClient, &argoWorkflow, taskName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow pods: %w", err)
	}

	// Get workflow plane resource
	workflowPlane, err := s.resolveWorkflowPlane(ctx, namespaceName, workflowPlaneRef)
	if err != nil {
		logger.Error("Failed to get workflow plane", "error", err)
		return nil, fmt.Errorf("failed to get workflow plane: %w", err)
	}

	// Get events from pods and convert to structured format
	allEventEntries := make([]models.WorkflowRunEventEntry, 0)
	for _, pod := range pods {
		podEvents, err := s.getArgoWorkflowPodEvents(ctx, workflowPlane, &pod)
		if err != nil {
			logger.Warn("Failed to get events from pod", "pod", pod.Name, "error", err)
			// Continue with other pods instead of failing completely
			continue
		}

		for _, event := range podEvents.Items {
			var timestamp time.Time
			if !event.EventTime.IsZero() {
				timestamp = event.EventTime.Time
			} else if !event.FirstTimestamp.IsZero() {
				timestamp = event.FirstTimestamp.Time
			} else if !event.LastTimestamp.IsZero() {
				timestamp = event.LastTimestamp.Time
			} else {
				// Skip events with no real timestamp to avoid misleading time values
				continue
			}

			allEventEntries = append(allEventEntries, models.WorkflowRunEventEntry{
				Timestamp: timestamp.Format(time.RFC3339),
				Type:      event.Type,
				Reason:    event.Reason,
				Message:   event.Message,
			})
		}
	}

	sort.SliceStable(allEventEntries, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, allEventEntries[i].Timestamp)
		tj, _ := time.Parse(time.RFC3339, allEventEntries[j].Timestamp)
		return ti.Before(tj)
	})

	return allEventEntries, nil
}

// getArgoWorkflowPods finds pods for a workflow and optionally a specific task (strict matching for events).
func (s *workflowRunService) getArgoWorkflowPods(ctx context.Context, wpClient client.Client, workflow *argoproj.Workflow, taskName string) ([]corev1.Pod, error) {
	return s.listAndFilterWorkflowPods(ctx, wpClient, workflow, taskName, false)
}

// getArgoWorkflowPodEvents retrieves events for a pod using the gateway client.
func (s *workflowRunService) getArgoWorkflowPodEvents(ctx context.Context, workflowPlane *openchoreov1alpha1.WorkflowPlane, pod *corev1.Pod) (*corev1.EventList, error) {
	if s.gwClient == nil {
		return nil, fmt.Errorf("gateway client is not configured")
	}

	body, err := s.gwClient.GetPodEventsFromPlane(ctx, "workflowplane", workflowPlane.Spec.PlaneID, workflowPlane.Namespace, workflowPlane.Name,
		&gatewayClient.PodReference{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod events: %w", err)
	}

	var eventList corev1.EventList
	if err := json.Unmarshal(body, &eventList); err != nil {
		return nil, fmt.Errorf("failed to decode events response: %w", err)
	}

	return &eventList, nil
}

// workflowRunStatusPending and related constants describe workflow run statuses.
const (
	workflowRunStatusPending   = "Pending"
	workflowRunStatusRunning   = "Running"
	workflowRunStatusSucceeded = "Succeeded"
	workflowRunStatusFailed    = "Failed"
)

// GetWorkflowRunStatus retrieves the status and step information for a specific WorkflowRun.
func (s *workflowRunService) GetWorkflowRunStatus(ctx context.Context, namespaceName, runName string) (*models.WorkflowRunStatusResponse, error) {
	logger := s.logger.With("namespace", namespaceName, "run", runName)
	logger.Debug("Getting workflow run status")

	wfRun := &openchoreov1alpha1.WorkflowRun{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      runName,
		Namespace: namespaceName,
	}, wfRun); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Warn("WorkflowRun not found")
			return nil, ErrWorkflowRunNotFound
		}
		logger.Error("Failed to get WorkflowRun", "error", err)
		return nil, fmt.Errorf("failed to get WorkflowRun: %w", err)
	}

	overallStatus := computeWorkflowRunStatus(wfRun.Status.Conditions)

	steps := make([]models.WorkflowStepStatus, 0, len(wfRun.Status.Tasks))
	for _, task := range wfRun.Status.Tasks {
		step := models.WorkflowStepStatus{
			Name:  task.Name,
			Phase: task.Phase,
		}
		if task.StartedAt != nil {
			startedAt := task.StartedAt.Time
			step.StartedAt = &startedAt
		}
		if task.CompletedAt != nil {
			completedAt := task.CompletedAt.Time
			step.FinishedAt = &completedAt
		}
		steps = append(steps, step)
	}

	hasLiveObservability := s.argoWorkflowExists(ctx, namespaceName, wfRun)

	return &models.WorkflowRunStatusResponse{
		Status:               overallStatus,
		Steps:                steps,
		HasLiveObservability: hasLiveObservability,
	}, nil
}

// argoWorkflowExists checks whether the Argo Workflow referenced by the given WorkflowRun
// still exists on the workflow plane. Returns true if it exists.
func (s *workflowRunService) argoWorkflowExists(ctx context.Context, namespaceName string, wfRun *openchoreov1alpha1.WorkflowRun) bool {
	runReference := wfRun.Status.RunReference
	if runReference == nil || runReference.Name == "" || runReference.Namespace == "" {
		return false
	}

	workflowPlaneRef, err := s.resolveWorkflowPlaneRef(ctx, namespaceName, wfRun.Spec.Workflow)
	if err != nil {
		s.logger.Debug("Failed to resolve workflow plane ref for existence check", "error", err)
		return false
	}

	wpClient, err := s.getWorkflowPlaneClient(ctx, namespaceName, workflowPlaneRef)
	if err != nil {
		s.logger.Debug("Failed to get workflow plane client for workflow existence check", "error", err)
		return false
	}

	var argoWorkflow argoproj.Workflow
	if err := wpClient.Get(ctx, types.NamespacedName{
		Name:      runReference.Name,
		Namespace: runReference.Namespace,
	}, &argoWorkflow); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false
		}
		s.logger.Debug("Failed to check argo workflow existence on workflow plane", "error", err)
		return false
	}

	return true
}

// computeWorkflowRunStatus determines the user-friendly status from workflow run conditions.
func computeWorkflowRunStatus(conditions []metav1.Condition) string {
	if len(conditions) == 0 {
		return workflowRunStatusPending
	}

	for _, condition := range conditions {
		if condition.Type == "WorkflowFailed" && condition.Status == metav1.ConditionTrue {
			return workflowRunStatusFailed
		}
	}

	for _, condition := range conditions {
		if condition.Type == "WorkflowSucceeded" && condition.Status == metav1.ConditionTrue {
			return workflowRunStatusSucceeded
		}
	}

	for _, condition := range conditions {
		if condition.Type == "WorkflowRunning" && condition.Status == metav1.ConditionTrue {
			return workflowRunStatusRunning
		}
	}

	return workflowRunStatusPending
}

// TriggerWorkflow creates a new WorkflowRun from a component's workflow configuration.
// This is used by both the authorized API handler path and the webhook path (unauthz).
func (s *workflowRunService) TriggerWorkflow(ctx context.Context, namespaceName, projectName, componentName, commit string) (*models.WorkflowRunTriggerResponse, error) {
	s.logger.Debug("Triggering component workflow", "namespace", namespaceName, "project", projectName, "component", componentName, "commit", commit)

	// Retrieve component
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}, &component); err != nil {
		s.logger.Error("Failed to get component", "error", err, "namespace", namespaceName, "component", componentName)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Derive the canonical project from the component's owner and validate against the caller.
	canonicalProject := component.Spec.Owner.ProjectName
	if projectName != "" && projectName != canonicalProject {
		return nil, fmt.Errorf("project %q does not match component owner project %q", projectName, canonicalProject)
	}
	projectName = canonicalProject

	// Check if component has workflow configuration
	if component.Spec.Workflow == nil || component.Spec.Workflow.Name == "" {
		s.logger.Error("Component does not have a workflow configured", "component", componentName)
		return nil, fmt.Errorf("component %s does not have a workflow configured", componentName)
	}

	// Fetch the Workflow or ClusterWorkflow CR to get the schema
	var workflowParameters *openchoreov1alpha1.SchemaSection
	if component.Spec.Workflow.Kind == openchoreov1alpha1.WorkflowRefKindClusterWorkflow {
		cw := &openchoreov1alpha1.ClusterWorkflow{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{
			Name: component.Spec.Workflow.Name,
		}, cw); err != nil {
			s.logger.Error("Failed to get ClusterWorkflow", "error", err, "workflow", component.Spec.Workflow.Name)
			return nil, fmt.Errorf("failed to get ClusterWorkflow %s: %w", component.Spec.Workflow.Name, err)
		}
		workflowParameters = cw.Spec.Parameters
	} else {
		workflow := &openchoreov1alpha1.Workflow{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{
			Name:      component.Spec.Workflow.Name,
			Namespace: namespaceName,
		}, workflow); err != nil {
			s.logger.Error("Failed to get workflow", "error", err, "workflow", component.Spec.Workflow.Name)
			return nil, fmt.Errorf("failed to get workflow %s: %w", component.Spec.Workflow.Name, err)
		}
		workflowParameters = workflow.Spec.Parameters
	}

	// Extract parameter paths from x-openchoreo-component-repository schema extensions
	paramMap, err := controller.ExtractComponentRepositoryPaths(workflowParameters.GetRaw())
	if err != nil {
		s.logger.Error("Failed to extract component repository paths from workflow schema", "error", err, "workflow", component.Spec.Workflow.Name)
		return nil, fmt.Errorf("failed to extract component repository paths from workflow %s schema: %w", component.Spec.Workflow.Name, err)
	}

	// Validate that repoUrl is configured in the component parameters.
	if repoURLPath, ok := paramMap["url"]; ok {
		repoURL, err := getNestedStringInParams(component.Spec.Workflow.Parameters, repoURLPath)
		if err != nil {
			s.logger.Error("Failed to read repository URL from component parameters", "error", err, "path", repoURLPath, "component", componentName)
			return nil, fmt.Errorf("failed to read repository URL for component %s at path %s: %w", componentName, repoURLPath, err)
		}
		if repoURL == "" {
			s.logger.Error("Repository URL is empty in component parameters", "component", componentName)
			return nil, fmt.Errorf("component %s has an empty repository URL configured", componentName)
		}
	}

	// Validate commit SHA format if provided
	if commit != "" {
		commitPattern := regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
		if !commitPattern.MatchString(commit) {
			return nil, ErrInvalidCommitSHA
		}
	}

	// Start with the component's existing parameters
	parameters := component.Spec.Workflow.Parameters

	// Inject commit SHA into parameters at the mapped path if a commit mapping exists
	if commit != "" {
		if commitPath, ok := paramMap["commit"]; ok {
			updatedParams, err := setNestedStringInParams(parameters, commitPath, commit)
			if err != nil {
				return nil, fmt.Errorf("failed to inject commit into workflow parameters: %w", err)
			}
			parameters = updatedParams
		}
	}

	// Generate a unique workflow run name
	workflowRunName, err := generateWorkflowRunName(componentName)
	if err != nil {
		s.logger.Error("Failed to generate workflow run name", "error", err)
		return nil, fmt.Errorf("failed to generate workflow run name: %w", err)
	}

	// Create the WorkflowRun CR
	workflowRun := &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workflowRunName,
			Namespace: namespaceName,
			Labels: map[string]string{
				ocLabels.LabelKeyProjectName:   projectName,
				ocLabels.LabelKeyComponentName: componentName,
			},
		},
		Spec: openchoreov1alpha1.WorkflowRunSpec{
			Workflow: openchoreov1alpha1.WorkflowRunConfig{
				Kind:       component.Spec.Workflow.Kind,
				Name:       component.Spec.Workflow.Name,
				Parameters: parameters,
			},
		},
	}

	if err := s.k8sClient.Create(ctx, workflowRun); err != nil {
		if apierrors.IsInvalid(err) {
			var statusErr *apierrors.StatusError
			if errors.As(err, &statusErr) && statusErr.ErrStatus.Details != nil {
				for _, cause := range statusErr.ErrStatus.Details.Causes {
					if strings.Contains(cause.Field, "commit") {
						s.logger.Warn("Commit SHA validation failed", "error", cause.Message, "field", cause.Field)
						return nil, ErrInvalidCommitSHA
					}
				}
			}
		}
		s.logger.Error("Failed to create workflow run", "error", err)
		return nil, fmt.Errorf("failed to create workflow run: %w", err)
	}

	s.logger.Info("Workflow run created successfully", "workflow", workflowRunName, "component", componentName, "commit", commit)

	return &models.WorkflowRunTriggerResponse{
		Name:          workflowRun.Name,
		UUID:          string(workflowRun.UID),
		ComponentName: componentName,
		ProjectName:   projectName,
		NamespaceName: namespaceName,
		Commit:        commit,
		Status:        workflowRunStatusPending,
		CreatedAt:     workflowRun.CreationTimestamp.Time,
	}, nil
}

// generateWorkflowRunName generates a unique name for the workflow run.
func generateWorkflowRunName(baseName string) (string, error) {
	bytes := make([]byte, 4) // 8 characters hex string
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random suffix: %w", err)
	}
	suffix := hex.EncodeToString(bytes)

	runName := fmt.Sprintf("%s-run-%s", baseName, suffix)

	// Ensure the name doesn't exceed Kubernetes name limits (63 characters)
	if len(runName) > 63 {
		maxBaseLen := 63 - len("-run-") - 8 // 8 for hex suffix
		if maxBaseLen > 0 {
			runName = fmt.Sprintf("%s-run-%s", baseName[:maxBaseLen], suffix)
		} else {
			return "", fmt.Errorf("base name is too long to generate valid run name")
		}
	}

	return runName, nil
}

// getNestedStringInParams navigates a runtime.RawExtension JSON blob using a dotted path
// and returns the string value. The leading "parameters." prefix is stripped if present.
func getNestedStringInParams(raw *runtime.RawExtension, dottedPath string) (string, error) {
	if raw == nil || raw.Raw == nil {
		return "", fmt.Errorf("parameters is nil")
	}

	path := strings.TrimPrefix(dottedPath, "parameters.")

	var data map[string]interface{}
	if err := json.Unmarshal(raw.Raw, &data); err != nil {
		return "", fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	parts := strings.Split(path, ".")
	current := interface{}(data)
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("path %s: expected object at %s", dottedPath, part)
		}
		current, ok = m[part]
		if !ok {
			return "", fmt.Errorf("path %s: key %s not found", dottedPath, part)
		}
	}

	str, ok := current.(string)
	if !ok {
		return "", fmt.Errorf("path %s: value is not a string", dottedPath)
	}
	return str, nil
}

// setNestedStringInParams sets a string value at the given dotted path in a runtime.RawExtension.
// The leading "parameters." prefix is stripped if present.
func setNestedStringInParams(raw *runtime.RawExtension, dottedPath, value string) (*runtime.RawExtension, error) {
	if raw == nil || raw.Raw == nil {
		return nil, fmt.Errorf("parameters is nil")
	}

	path := strings.TrimPrefix(dottedPath, "parameters.")

	var data map[string]interface{}
	if err := json.Unmarshal(raw.Raw, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	parts := strings.Split(path, ".")
	current := data
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part]
		if !ok {
			newObj := make(map[string]interface{})
			current[part] = newObj
			current = newObj
			continue
		}
		m, ok := next.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("path %s: expected object at %s", dottedPath, part)
		}
		current = m
	}

	current[parts[len(parts)-1]] = value

	rawBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameters: %w", err)
	}

	return &runtime.RawExtension{Raw: rawBytes}, nil
}
