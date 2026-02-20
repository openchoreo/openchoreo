// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// WorkflowRunServiceInterface defines the operations available on WorkflowRun resources.
// It is implemented by WorkflowRunService and may be replaced with a mock in tests.
type WorkflowRunServiceInterface interface {
	ListWorkflowRuns(ctx context.Context, namespaceName string) ([]*models.WorkflowRunResponse, error)
	GetWorkflowRun(ctx context.Context, namespaceName, runName string) (*models.WorkflowRunResponse, error)
	GetWorkflowRunStatus(ctx context.Context, namespaceName, runName, gatewayURL string) (*models.ComponentWorkflowRunStatusResponse, error)
	CreateWorkflowRun(ctx context.Context, namespaceName string, req *models.CreateWorkflowRunRequest) (*models.WorkflowRunResponse, error)
	GetWorkflowRunLogs(ctx context.Context, namespaceName, runName, stepName, gatewayURL string, sinceSeconds *int64) ([]models.ComponentWorkflowRunLogEntry, error)
	GetWorkflowRunEvents(ctx context.Context, namespaceName, runName, stepName, gatewayURL string) ([]models.ComponentWorkflowRunEventEntry, error)
}

// WorkflowRunService handles WorkflowRun-related business logic
type WorkflowRunService struct {
	k8sClient         client.Client
	logger            *slog.Logger
	authzPDP          authz.PDP
	buildPlaneService *BuildPlaneService
	gwClient          *gatewayClient.Client
}

// NewWorkflowRunService creates a new WorkflowRun service
func NewWorkflowRunService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP, buildPlaneService *BuildPlaneService, gwClient *gatewayClient.Client) *WorkflowRunService {
	return &WorkflowRunService{
		k8sClient:         k8sClient,
		logger:            logger,
		authzPDP:          authzPDP,
		buildPlaneService: buildPlaneService,
		gwClient:          gwClient,
	}
}

// ListWorkflowRuns lists all WorkflowRuns in the given namespace
func (s *WorkflowRunService) ListWorkflowRuns(ctx context.Context, namespaceName string) ([]*models.WorkflowRunResponse, error) {
	s.logger.Debug("Listing WorkflowRuns", "namespace", namespaceName)

	var wfRunList openchoreov1alpha1.WorkflowRunList
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}

	if err := s.k8sClient.List(ctx, &wfRunList, listOpts...); err != nil {
		s.logger.Error("Failed to list WorkflowRuns", "error", err)
		return nil, fmt.Errorf("failed to list WorkflowRuns: %w", err)
	}

	wfRuns := make([]*models.WorkflowRunResponse, 0, len(wfRunList.Items))
	for i := range wfRunList.Items {
		// Authorization check
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewWorkflowRun, ResourceTypeWorkflowRun, wfRunList.Items[i].Name,
			authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				// Skip unauthorized items
				s.logger.Debug("Skipping unauthorized workflow run", "namespace", namespaceName, "workflowRun", wfRunList.Items[i].Name)
				continue
			}
			return nil, err
		}
		wfRuns = append(wfRuns, s.toWorkflowRunResponse(&wfRunList.Items[i]))
	}

	s.logger.Debug("Listed WorkflowRuns", "namespace", namespaceName, "count", len(wfRuns))
	return wfRuns, nil
}

// GetWorkflowRun retrieves a specific WorkflowRun
func (s *WorkflowRunService) GetWorkflowRun(ctx context.Context, namespaceName, runName string) (*models.WorkflowRunResponse, error) {
	s.logger.Debug("Getting WorkflowRun", "org", namespaceName, "run", runName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewWorkflowRun, ResourceTypeWorkflowRun, runName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	wfRun := &openchoreov1alpha1.WorkflowRun{}
	key := client.ObjectKey{
		Name:      runName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, wfRun); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("WorkflowRun not found", "org", namespaceName, "run", runName)
			return nil, ErrWorkflowRunNotFound
		}
		s.logger.Error("Failed to get WorkflowRun", "error", err)
		return nil, fmt.Errorf("failed to get WorkflowRun: %w", err)
	}

	return s.toWorkflowRunResponse(wfRun), nil
}

// GetWorkflowRunStatus retrieves the status and step information for a specific WorkflowRun
func (s *WorkflowRunService) GetWorkflowRunStatus(ctx context.Context, namespaceName, runName, gatewayURL string) (*models.ComponentWorkflowRunStatusResponse, error) {
	logger := s.logger.With("namespace", namespaceName, "run", runName)
	logger.Debug("Getting workflow run status")

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewWorkflowRun, ResourceTypeWorkflowRun, runName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

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

	overallStatus := getWorkflowRunStatus(wfRun.Status.Conditions)

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

	hasLiveObservability := s.buildPlaneService.ArgoWorkflowExists(ctx, namespaceName, gatewayURL, wfRun.Status.RunReference)

	return &models.ComponentWorkflowRunStatusResponse{
		Status:               overallStatus,
		Steps:                steps,
		HasLiveObservability: hasLiveObservability,
	}, nil
}

// CreateWorkflowRun creates a new WorkflowRun
func (s *WorkflowRunService) CreateWorkflowRun(ctx context.Context, namespaceName string, req *models.CreateWorkflowRunRequest) (*models.WorkflowRunResponse, error) {
	s.logger.Debug("Creating WorkflowRun", "org", namespaceName, "workflow", req.WorkflowName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateWorkflowRun, ResourceTypeWorkflowRun, "",
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	// Verify the referenced workflow exists
	workflow := &openchoreov1alpha1.Workflow{}
	workflowKey := client.ObjectKey{
		Name:      req.WorkflowName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, workflowKey, workflow); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Referenced workflow not found", "org", namespaceName, "workflow", req.WorkflowName)
			return nil, ErrWorkflowNotFound
		}
		s.logger.Error("Failed to get referenced workflow", "error", err)
		return nil, fmt.Errorf("failed to get referenced workflow: %w", err)
	}

	// Generate a unique name for the workflow run
	runName, err := s.generateWorkflowRunName(req.WorkflowName)
	if err != nil {
		s.logger.Error("Failed to generate workflow run name", "error", err)
		return nil, fmt.Errorf("failed to generate workflow run name: %w", err)
	}

	// Convert parameters to runtime.RawExtension
	var parametersRaw *runtime.RawExtension
	if req.Parameters != nil {
		rawBytes, err := marshalToRawExtension(req.Parameters)
		if err != nil {
			s.logger.Error("Failed to marshal parameters", "error", err)
			return nil, fmt.Errorf("failed to marshal parameters: %w", err)
		}
		parametersRaw = rawBytes
	}

	// Create the WorkflowRun
	wfRun := &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      runName,
			Namespace: namespaceName,
		},
		Spec: openchoreov1alpha1.WorkflowRunSpec{
			Workflow: openchoreov1alpha1.WorkflowRunConfig{
				Name:       req.WorkflowName,
				Parameters: parametersRaw,
			},
		},
	}

	if err := s.k8sClient.Create(ctx, wfRun); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("WorkflowRun already exists", "org", namespaceName, "run", runName)
			return nil, ErrWorkflowRunAlreadyExists
		}
		s.logger.Error("Failed to create WorkflowRun", "error", err)
		return nil, fmt.Errorf("failed to create WorkflowRun: %w", err)
	}

	s.logger.Debug("Created WorkflowRun successfully", "org", namespaceName, "run", runName, "workflow", req.WorkflowName)
	return s.toWorkflowRunResponse(wfRun), nil
}

// generateWorkflowRunName generates a unique name for the workflow run
func (s *WorkflowRunService) generateWorkflowRunName(workflowName string) (string, error) {
	// Generate a random suffix
	bytes := make([]byte, 4) // 8 characters hex string
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random suffix: %w", err)
	}
	suffix := hex.EncodeToString(bytes)

	// Create name: workflowName-run-suffix
	runName := fmt.Sprintf("%s-run-%s", workflowName, suffix)

	// Ensure the name doesn't exceed Kubernetes name limits (63 characters)
	if len(runName) > 63 {
		// Truncate workflow name if needed
		maxWorkflowNameLen := 63 - len("-run-") - 8 // 8 for hex suffix
		if maxWorkflowNameLen > 0 {
			truncatedWorkflowName := workflowName[:maxWorkflowNameLen]
			runName = fmt.Sprintf("%s-run-%s", truncatedWorkflowName, suffix)
		} else {
			return "", fmt.Errorf("workflow name is too long to generate valid run name")
		}
	}

	return runName, nil
}

// toWorkflowRunResponse converts a WorkflowRun CRD to the API response model
func (s *WorkflowRunService) toWorkflowRunResponse(wfRun *openchoreov1alpha1.WorkflowRun) *models.WorkflowRunResponse {
	response := &models.WorkflowRunResponse{
		Name:          wfRun.Name,
		WorkflowName:  wfRun.Spec.Workflow.Name,
		NamespaceName: wfRun.Namespace,
		CreatedAt:     wfRun.CreationTimestamp.Time,
	}

	// Set UUID if available
	if wfRun.UID != "" {
		response.UUID = string(wfRun.UID)
	}

	// Extract status from conditions using priority order
	response.Status = getWorkflowRunStatus(wfRun.Status.Conditions)
	response.Phase = response.Status

	// Set FinishedAt from WorkflowCompleted condition when completed
	for _, condition := range wfRun.Status.Conditions {
		if condition.Type == "WorkflowCompleted" && condition.Status == metav1.ConditionTrue {
			response.FinishedAt = &condition.LastTransitionTime.Time
			break
		}
	}

	// Extract parameters if available
	if wfRun.Spec.Workflow.Parameters != nil {
		params, err := unmarshalRawExtension(wfRun.Spec.Workflow.Parameters)
		if err == nil {
			response.Parameters = params
		}
	}

	return response
}

// getWorkflowRunStatus determines the user-friendly status from workflow run conditions
func getWorkflowRunStatus(conditions []metav1.Condition) string {
	if len(conditions) == 0 {
		return WorkflowRunStatusPending
	}

	for _, condition := range conditions {
		if condition.Type == "WorkflowFailed" && condition.Status == metav1.ConditionTrue {
			return WorkflowRunStatusFailed
		}
	}

	for _, condition := range conditions {
		if condition.Type == "WorkflowSucceeded" && condition.Status == metav1.ConditionTrue {
			return WorkflowRunStatusSucceeded
		}
	}

	for _, condition := range conditions {
		if condition.Type == "WorkflowRunning" && condition.Status == metav1.ConditionTrue {
			return WorkflowRunStatusRunning
		}
	}

	return WorkflowRunStatusPending
}

// marshalToRawExtension marshals a map to runtime.RawExtension
func marshalToRawExtension(data map[string]interface{}) (*runtime.RawExtension, error) {
	if data == nil {
		return nil, nil
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data to JSON: %w", err)
	}

	return &runtime.RawExtension{
		Raw: bytes,
	}, nil
}

// unmarshalRawExtension unmarshals runtime.RawExtension to a map
func unmarshalRawExtension(raw *runtime.RawExtension) (map[string]interface{}, error) {
	if raw == nil || raw.Raw == nil {
		return nil, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw.Raw, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal raw extension: %w", err)
	}

	return result, nil
}

// GetWorkflowRunLogs retrieves logs from a workflow run
func (s *WorkflowRunService) GetWorkflowRunLogs(ctx context.Context, namespaceName, runName, stepName, gatewayURL string, sinceSeconds *int64) ([]models.ComponentWorkflowRunLogEntry, error) {
	logger := s.logger.With("namespace", namespaceName, "run", runName, "step", stepName, "sinceSeconds", sinceSeconds)
	logger.Debug("Getting workflow run logs")

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewWorkflowRun, ResourceTypeWorkflowRun, runName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

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

	// Get logs through the build plane client (Argo specific)
	// TODO: Extend to support other build engines (eg. Jenkins)
	return s.getArgoWorkflowRunLogs(ctx, namespaceName, gatewayURL, workflowRun.Status.RunReference, stepName, sinceSeconds)
}

// getArgoWorkflowRunLogs retrieves logs from an Argo Workflow run
func (s *WorkflowRunService) getArgoWorkflowRunLogs(
	ctx context.Context,
	namespaceName string,
	gatewayURL string,
	runReference *openchoreov1alpha1.ResourceReference,
	stepName string,
	sinceSeconds *int64,
) ([]models.ComponentWorkflowRunLogEntry, error) {
	logger := s.logger.With("namespace", namespaceName, "runReference", runReference, "step", stepName, "sinceSeconds", sinceSeconds)
	logger.Debug("Getting Argo workflow run logs")

	// Get build plane client
	bpClient, err := s.buildPlaneService.GetBuildPlaneClient(ctx, namespaceName, gatewayURL)
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

	// Get pods for the workflow/step
	pods, err := s.getArgoWorkflowPodsForLogs(ctx, bpClient, &argoWorkflow, stepName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow pods: %w", err)
	}

	// Get build plane resource
	buildPlane, err := s.buildPlaneService.GetBuildPlane(ctx, namespaceName)
	if err != nil {
		logger.Error("Failed to get build plane", "error", err)
		return nil, fmt.Errorf("failed to get build plane: %w", err)
	}

	// Get logs from pods and convert to structured format
	allLogEntries := make([]models.ComponentWorkflowRunLogEntry, 0)
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
					allLogEntries = append(allLogEntries, models.ComponentWorkflowRunLogEntry{
						Timestamp: timestampCandidate,
						Log:       trimmedLine[spaceIndex+1:],
					})
					continue
				}
				if _, err := time.Parse(time.RFC3339Nano, timestampCandidate); err == nil {
					allLogEntries = append(allLogEntries, models.ComponentWorkflowRunLogEntry{
						Timestamp: timestampCandidate,
						Log:       trimmedLine[spaceIndex+1:],
					})
					continue
				}
			}

			allLogEntries = append(allLogEntries, models.ComponentWorkflowRunLogEntry{
				Timestamp: "",
				Log:       trimmedLine,
			})
		}
	}

	return allLogEntries, nil
}

// listAndFilterWorkflowPods lists all pods for a workflow and filters them by stepName.
// When allowSubstring is true, pods whose node-name annotation is absent are also matched
// by checking whether the pod name contains stepName (used for log retrieval).
func (s *WorkflowRunService) listAndFilterWorkflowPods(ctx context.Context, bpClient client.Client, workflow *argoproj.Workflow, stepName string, allowSubstring bool) ([]corev1.Pod, error) {
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

	if stepName != "" {
		filteredPods := make([]corev1.Pod, 0)
		for _, pod := range podList.Items {
			nodeName := pod.Annotations["workflows.argoproj.io/node-name"]
			if nodeName == stepName || (allowSubstring && nodeName == "" && strings.Contains(pod.Name, stepName)) {
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

// getArgoWorkflowPodsForLogs finds pods for a workflow and optionally a specific step (for log retrieval).
// Falls back to a pod-name substring match when the node-name annotation is absent.
func (s *WorkflowRunService) getArgoWorkflowPodsForLogs(ctx context.Context, bpClient client.Client, workflow *argoproj.Workflow, stepName string) ([]corev1.Pod, error) {
	return s.listAndFilterWorkflowPods(ctx, bpClient, workflow, stepName, true)
}

// getArgoWorkflowPodLogs retrieves logs from an Argo Workflow pod using the gateway client
func (s *WorkflowRunService) getArgoWorkflowPodLogs(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane, pod *corev1.Pod, sinceSeconds *int64) (string, error) {
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

// GetWorkflowRunEvents retrieves events from a workflow run
func (s *WorkflowRunService) GetWorkflowRunEvents(ctx context.Context, namespaceName, runName, stepName, gatewayURL string) ([]models.ComponentWorkflowRunEventEntry, error) {
	logger := s.logger.With("namespace", namespaceName, "run", runName, "step", stepName)
	logger.Debug("Getting workflow run events")

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewWorkflowRun, ResourceTypeWorkflowRun, runName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

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

	// Get events through the build plane client (Argo specific)
	// TODO: Extend to support other build engines (eg. Jenkins)
	return s.getArgoWorkflowRunEvents(ctx, namespaceName, gatewayURL, workflowRun.Status.RunReference, stepName)
}

// getArgoWorkflowRunEvents retrieves events from an Argo Workflow run
func (s *WorkflowRunService) getArgoWorkflowRunEvents(
	ctx context.Context,
	namespaceName string,
	gatewayURL string,
	runReference *openchoreov1alpha1.ResourceReference,
	stepName string,
) ([]models.ComponentWorkflowRunEventEntry, error) {
	logger := s.logger.With("namespace", namespaceName, "runReference", runReference, "step", stepName)
	logger.Debug("Getting Argo workflow run events")

	// Get build plane client
	bpClient, err := s.buildPlaneService.GetBuildPlaneClient(ctx, namespaceName, gatewayURL)
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

	// Get pods for the workflow/step
	pods, err := s.getArgoWorkflowPods(ctx, bpClient, &argoWorkflow, stepName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow pods: %w", err)
	}

	// Get build plane resource
	buildPlane, err := s.buildPlaneService.GetBuildPlane(ctx, namespaceName)
	if err != nil {
		logger.Error("Failed to get build plane", "error", err)
		return nil, fmt.Errorf("failed to get build plane: %w", err)
	}

	// Get events from pods and convert to structured format
	allEventEntries := make([]models.ComponentWorkflowRunEventEntry, 0)
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

			allEventEntries = append(allEventEntries, models.ComponentWorkflowRunEventEntry{
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

// getArgoWorkflowPods finds pods for a workflow and optionally a specific step
func (s *WorkflowRunService) getArgoWorkflowPods(ctx context.Context, bpClient client.Client, workflow *argoproj.Workflow, stepName string) ([]corev1.Pod, error) {
	return s.listAndFilterWorkflowPods(ctx, bpClient, workflow, stepName, false)
}

// getArgoWorkflowPodEvents retrieves events for a pod using the gateway client
func (s *WorkflowRunService) getArgoWorkflowPodEvents(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane, pod *corev1.Pod) (*corev1.EventList, error) {
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
