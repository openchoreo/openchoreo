// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	ocLabels "github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// workflowRunService handles workflow run business logic without authorization checks.
type workflowRunService struct {
	k8sClient   client.Client
	bpClientMgr *kubernetesClient.KubeMultiClientManager
	gwClient    *gatewayClient.Client
	logger      *slog.Logger
}

var _ Service = (*workflowRunService)(nil)

var workflowRunTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "WorkflowRun",
}

// NewService creates a new workflow run service without authorization.
func NewService(k8sClient client.Client, bpClientMgr *kubernetesClient.KubeMultiClientManager, gwClient *gatewayClient.Client, logger *slog.Logger) Service {
	return &workflowRunService{
		k8sClient:   k8sClient,
		bpClientMgr: bpClientMgr,
		gwClient:    gwClient,
		logger:      logger,
	}
}

func (s *workflowRunService) CreateWorkflowRun(ctx context.Context, namespaceName string, wfRun *openchoreov1alpha1.WorkflowRun) (*openchoreov1alpha1.WorkflowRun, error) {
	if wfRun == nil {
		return nil, fmt.Errorf("workflow run cannot be nil")
	}

	s.logger.Debug("Creating workflow run", "namespace", namespaceName, "name", wfRun.Name)

	// Verify the referenced workflow exists
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

// GetWorkflowRunLogs retrieves logs from a workflow run.
func (s *workflowRunService) GetWorkflowRunLogs(ctx context.Context, namespaceName, runName, taskName, gatewayURL string, sinceSeconds *int64) ([]models.WorkflowRunLogEntry, error) {
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

	return s.getArgoWorkflowRunLogs(ctx, namespaceName, gatewayURL, workflowRun.Status.RunReference, taskName, sinceSeconds)
}

// getArgoWorkflowRunLogs retrieves logs from an Argo Workflow run.
func (s *workflowRunService) getArgoWorkflowRunLogs(
	ctx context.Context,
	namespaceName string,
	gatewayURL string,
	runReference *openchoreov1alpha1.ResourceReference,
	taskName string,
	sinceSeconds *int64,
) ([]models.WorkflowRunLogEntry, error) {
	logger := s.logger.With("namespace", namespaceName, "runReference", runReference, "task", taskName, "sinceSeconds", sinceSeconds)
	logger.Debug("Getting Argo workflow run logs")

	// Get build plane client
	bpClient, err := s.getBuildPlaneClient(ctx, namespaceName, gatewayURL)
	if err != nil {
		logger.Error("Failed to get build plane client", "error", err)
		return nil, fmt.Errorf("failed to get build plane client: %w", err)
	}

	// Get Argo Workflow from build plane
	var argoWorkflow argoproj.Workflow
	if err := bpClient.Get(ctx, types.NamespacedName{
		Name:      runReference.Name,
		Namespace: runReference.Namespace,
	}, &argoWorkflow); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Warn("Argo workflow not found in build plane", "workflow", runReference.Name, "namespace", runReference.Namespace, "error", err)
			return nil, fmt.Errorf("argo workflow not found: %w", err)
		}
		logger.Error("Failed to get argo workflow", "error", err)
		return nil, fmt.Errorf("failed to get argo workflow: %w", err)
	}

	// Get pods for the workflow/task
	pods, err := s.getArgoWorkflowPodsForLogs(ctx, bpClient, &argoWorkflow, taskName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow pods: %w", err)
	}

	// Get build plane resource
	buildPlane, err := s.getBuildPlane(ctx, namespaceName)
	if err != nil {
		logger.Error("Failed to get build plane", "error", err)
		return nil, fmt.Errorf("failed to get build plane: %w", err)
	}

	// Get logs from pods and convert to structured format
	allLogEntries := make([]models.WorkflowRunLogEntry, 0)
	for _, pod := range pods {
		podLogs, err := s.getArgoWorkflowPodLogs(ctx, buildPlane, &pod, sinceSeconds)
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

// getBuildPlane retrieves the build plane for a namespace.
func (s *workflowRunService) getBuildPlane(ctx context.Context, namespaceName string) (*openchoreov1alpha1.BuildPlane, error) {
	var buildPlanes openchoreov1alpha1.BuildPlaneList
	if err := s.k8sClient.List(ctx, &buildPlanes, client.InNamespace(namespaceName)); err != nil {
		s.logger.Error("Failed to list build planes", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to list build planes: %w", err)
	}

	if len(buildPlanes.Items) == 0 {
		s.logger.Warn("No build planes found", "namespace", namespaceName)
		return nil, fmt.Errorf("no build planes found for namespace: %s", namespaceName)
	}

	return &buildPlanes.Items[0], nil
}

// getBuildPlaneClient creates and returns a Kubernetes client for the build plane cluster.
func (s *workflowRunService) getBuildPlaneClient(ctx context.Context, namespaceName string, gatewayURL string) (client.Client, error) {
	s.logger.Debug("Getting build plane client", "namespace", namespaceName)

	buildPlane, err := s.getBuildPlane(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane: %w", err)
	}

	buildPlaneClient, err := kubernetesClient.GetK8sClientFromBuildPlane(
		s.bpClientMgr,
		buildPlane,
		gatewayURL,
	)
	if err != nil {
		s.logger.Error("Failed to create build plane client", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to create build plane client: %w", err)
	}

	s.logger.Debug("Created build plane client", "namespace", namespaceName, "cluster", buildPlane.Name)
	return buildPlaneClient, nil
}

// listAndFilterWorkflowPods lists all pods for a workflow and filters them by taskName.
// When allowSubstring is true, pods whose node-name annotation is absent are also matched
// by checking whether the pod name contains taskName (used for log retrieval).
func (s *workflowRunService) listAndFilterWorkflowPods(ctx context.Context, bpClient client.Client, workflow *argoproj.Workflow, taskName string, allowSubstring bool) ([]corev1.Pod, error) {
	selector := labels.Set{
		"workflows.argoproj.io/workflow": workflow.Name,
	}

	var podList corev1.PodList
	if err := bpClient.List(ctx, &podList, client.InNamespace(workflow.Namespace), client.MatchingLabels(selector)); err != nil {
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
func (s *workflowRunService) getArgoWorkflowPodsForLogs(ctx context.Context, bpClient client.Client, workflow *argoproj.Workflow, taskName string) ([]corev1.Pod, error) {
	return s.listAndFilterWorkflowPods(ctx, bpClient, workflow, taskName, true)
}

// getArgoWorkflowPodLogs retrieves logs from an Argo Workflow pod using the gateway client.
func (s *workflowRunService) getArgoWorkflowPodLogs(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane, pod *corev1.Pod, sinceSeconds *int64) (string, error) {
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

		logs, err := s.gwClient.GetPodLogsFromPlane(ctx, "buildplane", buildPlane.Spec.PlaneID, buildPlane.Namespace, buildPlane.Name,
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
func (s *workflowRunService) GetWorkflowRunEvents(ctx context.Context, namespaceName, runName, taskName, gatewayURL string) ([]models.WorkflowRunEventEntry, error) {
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

	return s.getArgoWorkflowRunEvents(ctx, namespaceName, gatewayURL, workflowRun.Status.RunReference, taskName)
}

// getArgoWorkflowRunEvents retrieves events from an Argo Workflow run.
func (s *workflowRunService) getArgoWorkflowRunEvents(
	ctx context.Context,
	namespaceName string,
	gatewayURL string,
	runReference *openchoreov1alpha1.ResourceReference,
	taskName string,
) ([]models.WorkflowRunEventEntry, error) {
	logger := s.logger.With("namespace", namespaceName, "runReference", runReference, "task", taskName)
	logger.Debug("Getting Argo workflow run events")

	// Get build plane client
	bpClient, err := s.getBuildPlaneClient(ctx, namespaceName, gatewayURL)
	if err != nil {
		logger.Error("Failed to get build plane client", "error", err)
		return nil, fmt.Errorf("failed to get build plane client: %w", err)
	}

	// Get Argo Workflow from build plane
	var argoWorkflow argoproj.Workflow
	if err := bpClient.Get(ctx, types.NamespacedName{
		Name:      runReference.Name,
		Namespace: runReference.Namespace,
	}, &argoWorkflow); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Warn("Argo workflow not found in build plane", "workflow", runReference.Name, "namespace", runReference.Namespace)
			return nil, fmt.Errorf("argo workflow not found: %w", err)
		}
		logger.Error("Failed to get argo workflow", "error", err)
		return nil, fmt.Errorf("failed to get argo workflow: %w", err)
	}

	// Get pods for the workflow/task (strict matching for events)
	pods, err := s.getArgoWorkflowPods(ctx, bpClient, &argoWorkflow, taskName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow pods: %w", err)
	}

	// Get build plane resource
	buildPlane, err := s.getBuildPlane(ctx, namespaceName)
	if err != nil {
		logger.Error("Failed to get build plane", "error", err)
		return nil, fmt.Errorf("failed to get build plane: %w", err)
	}

	// Get events from pods and convert to structured format
	allEventEntries := make([]models.WorkflowRunEventEntry, 0)
	for _, pod := range pods {
		podEvents, err := s.getArgoWorkflowPodEvents(ctx, buildPlane, &pod)
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
func (s *workflowRunService) getArgoWorkflowPods(ctx context.Context, bpClient client.Client, workflow *argoproj.Workflow, taskName string) ([]corev1.Pod, error) {
	return s.listAndFilterWorkflowPods(ctx, bpClient, workflow, taskName, false)
}

// getArgoWorkflowPodEvents retrieves events for a pod using the gateway client.
func (s *workflowRunService) getArgoWorkflowPodEvents(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane, pod *corev1.Pod) (*corev1.EventList, error) {
	if s.gwClient == nil {
		return nil, fmt.Errorf("gateway client is not configured")
	}

	body, err := s.gwClient.GetPodEventsFromPlane(ctx, "buildplane", buildPlane.Spec.PlaneID, buildPlane.Namespace, buildPlane.Name,
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
