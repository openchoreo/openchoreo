// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/controller"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/schema/extractor"
)

// ComponentWorkflowService handles component workflow-related business logic
type ComponentWorkflowService struct {
	k8sClient         client.Client
	logger            *slog.Logger
	authzPDP          authz.PDP
	buildPlaneService *BuildPlaneService
	gwClient          *gatewayClient.Client
}

// NewComponentWorkflowService creates a new component workflow service
func NewComponentWorkflowService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP, buildPlaneService *BuildPlaneService, gwClient *gatewayClient.Client) *ComponentWorkflowService {
	return &ComponentWorkflowService{
		k8sClient:         k8sClient,
		logger:            logger,
		authzPDP:          authzPDP,
		buildPlaneService: buildPlaneService,
		gwClient:          gwClient,
	}
}

// AuthorizeCreate checks if the current user is authorized to create a ComponentWorkflow
func (s *ComponentWorkflowService) AuthorizeCreate(ctx context.Context, namespaceName, cwName string) error {
	return checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateComponentWorkflow,
		ResourceTypeComponentWorkflow, cwName, authz.ResourceHierarchy{Namespace: namespaceName})
}

// TriggerWorkflow creates a new ComponentWorkflowRun from a component's workflow configuration
func (s *ComponentWorkflowService) TriggerWorkflow(ctx context.Context, namespaceName, projectName, componentName, commit string) (*models.ComponentWorkflowResponse, error) {
	s.logger.Debug("Triggering component workflow", "namespace", namespaceName, "project", projectName, "component", componentName, "commit", commit)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateComponentWorkflow, ResourceTypeComponentWorkflow, componentName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	// Retrieve component and use that to create the workflow run
	var component openchoreov1alpha1.Component
	err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}, &component)

	if err != nil {
		s.logger.Error("Failed to get component", "error", err, "namespace", namespaceName, "project", projectName, "component", componentName)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Check if component has workflow configuration
	if component.Spec.Workflow == nil {
		s.logger.Error("Component does not have a workflow configured", "component", componentName)
		return nil, fmt.Errorf("component %s does not have a workflow configured", componentName)
	}

	// Extract system parameters from the component's workflow configuration
	var systemParams openchoreov1alpha1.SystemParametersValues
	if component.Spec.Workflow.SystemParameters.Repository.URL == "" {
		s.logger.Error("Component workflow does not have repository URL configured", "component", componentName)
		return nil, fmt.Errorf("component %s workflow does not have repository URL configured", componentName)
	}

	// Copy system parameters and update the commit
	systemParams = component.Spec.Workflow.SystemParameters

	// Validate commit SHA format if provided
	if commit != "" {
		// Git commit SHA validation: 7-40 hexadecimal characters
		commitPattern := regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
		if !commitPattern.MatchString(commit) {
			return nil, ErrInvalidCommitSHA
		}
	}

	systemParams.Repository.Revision.Commit = commit

	// Generate a unique workflow run name with short UUID
	uuid, err := generateShortUUID()
	if err != nil {
		s.logger.Error("Failed to generate UUID", "error", err)
		return nil, fmt.Errorf("failed to generate UUID: %w", err)
	}
	workflowRunName := fmt.Sprintf("%s-workflow-%s", componentName, uuid)

	// Create the ComponentWorkflowRun CR
	workflowRun := &openchoreov1alpha1.ComponentWorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workflowRunName,
			Namespace: namespaceName,
		},
		Spec: openchoreov1alpha1.ComponentWorkflowRunSpec{
			Owner: openchoreov1alpha1.ComponentWorkflowOwner{
				ProjectName:   projectName,
				ComponentName: componentName,
			},
			Workflow: openchoreov1alpha1.ComponentWorkflowRunConfig{
				Name:             component.Spec.Workflow.Name,
				SystemParameters: systemParams,
				Parameters:       component.Spec.Workflow.Parameters,
			},
		},
	}

	if err := s.k8sClient.Create(ctx, workflowRun); err != nil {
		// Check if this is a validation error from Kubernetes
		if apierrors.IsInvalid(err) {
			// Extract validation error details
			var statusErr *apierrors.StatusError
			if errors.As(err, &statusErr) && statusErr.ErrStatus.Details != nil {
				// Check if the error is related to commit SHA validation
				for _, cause := range statusErr.ErrStatus.Details.Causes {
					if strings.Contains(cause.Field, "commit") {
						s.logger.Warn("Commit SHA validation failed", "error", cause.Message, "field", cause.Field)
						return nil, ErrInvalidCommitSHA
					}
				}
			}
		}
		s.logger.Error("Failed to create component workflow run", "error", err)
		return nil, fmt.Errorf("failed to create component workflow run: %w", err)
	}

	s.logger.Info("Component workflow run created successfully", "workflow", workflowRunName, "component", componentName, "commit", commit)

	// Return a ComponentWorkflowResponse
	return &models.ComponentWorkflowResponse{
		Name:          workflowRun.Name,
		UUID:          string(workflowRun.UID),
		ComponentName: componentName,
		ProjectName:   projectName,
		NamespaceName: namespaceName,
		Commit:        commit,
		Status:        WorkflowRunStatusPending,
		CreatedAt:     workflowRun.CreationTimestamp.Time,
		Image:         "",
	}, nil
}

// ListComponentWorkflowRuns retrieves component workflow runs for a component using spec.owner fields
func (s *ComponentWorkflowService) ListComponentWorkflowRuns(ctx context.Context, namespaceName, projectName, componentName string) ([]models.ComponentWorkflowResponse, error) {
	s.logger.Debug("Listing component workflow runs", "namespace", namespaceName, "project", projectName, "component", componentName)

	var workflowRuns openchoreov1alpha1.ComponentWorkflowRunList
	err := s.k8sClient.List(ctx, &workflowRuns, client.InNamespace(namespaceName))
	if err != nil {
		s.logger.Error("Failed to list component workflow runs", "error", err)
		return nil, fmt.Errorf("failed to list component workflow runs: %w", err)
	}

	workflowResponses := make([]models.ComponentWorkflowResponse, 0, len(workflowRuns.Items))
	for _, workflowRun := range workflowRuns.Items {
		// Filter by spec.owner fields
		if workflowRun.Spec.Owner.ProjectName != projectName || workflowRun.Spec.Owner.ComponentName != componentName {
			continue
		}

		// Authorization check for each workflow run
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponentWorkflowRun, ResourceTypeComponentWorkflowRun, workflowRun.Name,
			authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized component workflow run", "namespace", namespaceName, "project", projectName, "component", componentName, "workflowRun", workflowRun.Name)
				continue
			}
			return nil, err
		}

		// Extract commit from the workflow system parameters
		commit := workflowRun.Spec.Workflow.SystemParameters.Repository.Revision.Commit
		if commit == "" {
			commit = "latest"
		}

		workflowResponses = append(workflowResponses, models.ComponentWorkflowResponse{
			Name:          workflowRun.Name,
			UUID:          string(workflowRun.UID),
			ComponentName: componentName,
			ProjectName:   projectName,
			NamespaceName: namespaceName,
			Commit:        commit,
			Status:        getComponentWorkflowStatus(workflowRun.Status.Conditions),
			CreatedAt:     workflowRun.CreationTimestamp.Time,
			Image:         workflowRun.Status.ImageStatus.Image,
		})
	}

	return workflowResponses, nil
}

// GetComponentWorkflowRun retrieves a specific component workflow run by name
func (s *ComponentWorkflowService) GetComponentWorkflowRun(ctx context.Context, namespaceName, projectName, componentName, runName string) (*models.ComponentWorkflowResponse, error) {
	s.logger.Debug("Getting component workflow run", "namespace", namespaceName, "project", projectName, "component", componentName, "run", runName)

	var workflowRun openchoreov1alpha1.ComponentWorkflowRun
	err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      runName,
		Namespace: namespaceName,
	}, &workflowRun)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component workflow run not found", "namespace", namespaceName, "run", runName)
			return nil, ErrComponentWorkflowRunNotFound
		}
		s.logger.Error("Failed to get component workflow run", "error", err)
		return nil, fmt.Errorf("failed to get component workflow run: %w", err)
	}

	// Verify the workflow run belongs to the specified component
	if workflowRun.Spec.Owner.ProjectName != projectName || workflowRun.Spec.Owner.ComponentName != componentName {
		s.logger.Warn("Component workflow run does not belong to the specified component",
			"namespace", namespaceName, "project", projectName, "component", componentName, "run", runName)
		return nil, ErrComponentWorkflowRunNotFound
	}

	// Extract commit from the workflow system parameters
	commit := workflowRun.Spec.Workflow.SystemParameters.Repository.Revision.Commit
	if commit == "" {
		commit = "latest"
	}

	// Build workflow configuration response
	workflowConfig := &models.ComponentWorkflowConfigResponse{
		Name: workflowRun.Spec.Workflow.Name,
		SystemParameters: &models.SystemParametersResponse{
			Repository: &models.RepositoryResponse{
				URL:     workflowRun.Spec.Workflow.SystemParameters.Repository.URL,
				AppPath: workflowRun.Spec.Workflow.SystemParameters.Repository.AppPath,
				Revision: &models.RepositoryRevisionResponse{
					Branch: workflowRun.Spec.Workflow.SystemParameters.Repository.Revision.Branch,
					Commit: workflowRun.Spec.Workflow.SystemParameters.Repository.Revision.Commit,
				},
			},
		},
	}

	// Parse parameters if present
	if workflowRun.Spec.Workflow.Parameters != nil && workflowRun.Spec.Workflow.Parameters.Raw != nil {
		var params map[string]any
		if err := yaml.Unmarshal(workflowRun.Spec.Workflow.Parameters.Raw, &params); err != nil {
			s.logger.Warn("Failed to parse workflow parameters", "error", err)
		} else {
			workflowConfig.Parameters = params
		}
	}

	return &models.ComponentWorkflowResponse{
		Name:          workflowRun.Name,
		UUID:          string(workflowRun.UID),
		NamespaceName: namespaceName,
		ProjectName:   projectName,
		ComponentName: componentName,
		Commit:        commit,
		Status:        getComponentWorkflowStatus(workflowRun.Status.Conditions),
		Image:         workflowRun.Status.ImageStatus.Image,
		Workflow:      workflowConfig,
		CreatedAt:     workflowRun.CreationTimestamp.Time,
	}, nil
}

// getComponentWorkflowStatus determines the user-friendly status from component workflow run conditions
func getComponentWorkflowStatus(workflowConditions []metav1.Condition) string {
	if len(workflowConditions) == 0 {
		return WorkflowRunStatusPending
	}

	// Check conditions in priority order
	// Similar to build workflow status logic
	for _, condition := range workflowConditions {
		if condition.Type == "WorkloadUpdated" && condition.Status == metav1.ConditionTrue {
			return WorkflowRunStatusCompleted
		}
	}

	for _, condition := range workflowConditions {
		if condition.Type == "WorkflowFailed" && condition.Status == metav1.ConditionTrue {
			return "Failed"
		}
	}

	for _, condition := range workflowConditions {
		if condition.Type == "WorkflowSucceeded" && condition.Status == metav1.ConditionTrue {
			return "Succeeded"
		}
	}

	for _, condition := range workflowConditions {
		if condition.Type == "WorkflowRunning" && condition.Status == metav1.ConditionTrue {
			return "Running"
		}
	}

	return WorkflowRunStatusPending
}

// ListComponentWorkflows lists all ComponentWorkflow templates in the given namespace
func (s *ComponentWorkflowService) ListComponentWorkflows(ctx context.Context, namespaceName string) ([]*models.WorkflowResponse, error) {
	s.logger.Debug("Listing ComponentWorkflow templates", "namespace", namespaceName)

	var cwfList openchoreov1alpha1.ComponentWorkflowList
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}

	if err := s.k8sClient.List(ctx, &cwfList, listOpts...); err != nil {
		s.logger.Error("Failed to list ComponentWorkflow templates", "error", err)
		return nil, fmt.Errorf("failed to list ComponentWorkflow templates: %w", err)
	}

	cwfs := make([]*models.WorkflowResponse, 0, len(cwfList.Items))
	for i := range cwfList.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponentWorkflow, ResourceTypeComponentWorkflow, cwfList.Items[i].Name,
			authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				// Skip unauthorized items
				s.logger.Debug("Skipping unauthorized component workflow", "namespace", namespaceName, "componentWorkflow", cwfList.Items[i].Name)
				continue
			}
			// Return other errors
			return nil, err
		}
		cwfs = append(cwfs, s.toComponentWorkflowResponse(&cwfList.Items[i]))
	}

	s.logger.Debug("Listed ComponentWorkflow templates", "namespace", namespaceName, "count", len(cwfs))
	return cwfs, nil
}

// GetComponentWorkflow retrieves a specific ComponentWorkflow template
func (s *ComponentWorkflowService) GetComponentWorkflow(ctx context.Context, namespaceName, cwfName string) (*models.WorkflowResponse, error) {
	s.logger.Debug("Getting ComponentWorkflow", "namespace", namespaceName, "name", cwfName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponentWorkflow, ResourceTypeComponentWorkflow, cwfName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	cwf := &openchoreov1alpha1.ComponentWorkflow{}
	key := client.ObjectKey{
		Name:      cwfName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, cwf); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ComponentWorkflow template not found", "namespace", namespaceName, "name", cwfName)
			return nil, ErrComponentWorkflowNotFound
		}
		s.logger.Error("Failed to get ComponentWorkflow template", "error", err)
		return nil, fmt.Errorf("failed to get ComponentWorkflow template: %w", err)
	}

	return s.toComponentWorkflowResponse(cwf), nil
}

// GetComponentWorkflowSchema retrieves the JSON schema for a ComponentWorkflow template
func (s *ComponentWorkflowService) GetComponentWorkflowSchema(ctx context.Context, namespaceName, cwfName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting ComponentWorkflow template schema", "namespace", namespaceName, "name", cwfName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponentWorkflow, ResourceTypeComponentWorkflow, cwfName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	cwf := &openchoreov1alpha1.ComponentWorkflow{}
	key := client.ObjectKey{
		Name:      cwfName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, cwf); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ComponentWorkflow template not found", "namespace", namespaceName, "name", cwfName)
			return nil, ErrComponentWorkflowNotFound
		}
		s.logger.Error("Failed to get ComponentWorkflow template", "error", err)
		return nil, fmt.Errorf("failed to get ComponentWorkflow template: %w", err)
	}

	// ComponentWorkflow has both systemParameters and parameters in the schema
	// We need to merge them into a single schema
	schemaMap := make(map[string]any)

	// Add systemParameters schema
	if cwf.Spec.Schema.SystemParameters.Repository.URL != "" {
		repoSchema := map[string]any{
			"url": cwf.Spec.Schema.SystemParameters.Repository.URL,
			"revision": map[string]any{
				"branch": cwf.Spec.Schema.SystemParameters.Repository.Revision.Branch,
				"commit": cwf.Spec.Schema.SystemParameters.Repository.Revision.Commit,
			},
			"appPath": cwf.Spec.Schema.SystemParameters.Repository.AppPath,
		}
		// Add secretRef if present in the schema
		if cwf.Spec.Schema.SystemParameters.Repository.SecretRef != "" {
			repoSchema["secretRef"] = cwf.Spec.Schema.SystemParameters.Repository.SecretRef
		}
		schemaMap["systemParameters"] = map[string]any{
			"repository": repoSchema,
		}
	}

	// Add developer parameters schema if present
	if cwf.Spec.Schema.Parameters != nil && cwf.Spec.Schema.Parameters.Raw != nil {
		var paramsMap map[string]any
		if err := yaml.Unmarshal(cwf.Spec.Schema.Parameters.Raw, &paramsMap); err != nil {
			return nil, fmt.Errorf("failed to extract parameters schema: %w", err)
		}
		schemaMap["parameters"] = paramsMap
	}

	def := schema.Definition{
		Schemas: []map[string]any{schemaMap},
		Options: extractor.Options{
			SkipDefaultValidation: true,
		},
	}

	jsonSchema, err := schema.ToJSONSchema(def)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved ComponentWorkflow template schema successfully", "namespace", namespaceName, "name", cwfName)
	return jsonSchema, nil
}

func (s *ComponentWorkflowService) toComponentWorkflowResponse(cwf *openchoreov1alpha1.ComponentWorkflow) *models.WorkflowResponse {
	return &models.WorkflowResponse{
		Name:        cwf.Name,
		DisplayName: cwf.Annotations[controller.AnnotationKeyDisplayName],
		Description: cwf.Annotations[controller.AnnotationKeyDescription],
		CreatedAt:   cwf.CreationTimestamp.Time,
	}
}

// generateShortUUID generates a short 8-character UUID for workflow naming.
func generateShortUUID() (string, error) {
	bytes := make([]byte, 4) // 4 bytes = 8 hex characters
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GetComponentWorkflowRunStatus retrieves the status of a component workflow run
func (s *ComponentWorkflowService) GetComponentWorkflowRunStatus(ctx context.Context, namespaceName, projectName, componentName, runName, gatewayURL string) (*models.ComponentWorkflowRunStatusResponse, error) {
	logger := s.logger.With("namespace", namespaceName, "project", projectName, "component", componentName, "run", runName)
	logger.Debug("Getting component workflow run status")

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponentWorkflowRun, ResourceTypeComponentWorkflowRun, runName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	// Get ComponentWorkflowRun
	var workflowRun openchoreov1alpha1.ComponentWorkflowRun
	err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      runName,
		Namespace: namespaceName,
	}, &workflowRun)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Warn("Component workflow run not found")
			return nil, ErrComponentWorkflowRunNotFound
		}
		logger.Error("Failed to get component workflow run", "error", err)
		return nil, fmt.Errorf("failed to get component workflow run: %w", err)
	}

	// Verify the workflow run belongs to the specified component
	if workflowRun.Spec.Owner.ProjectName != projectName || workflowRun.Spec.Owner.ComponentName != componentName {
		logger.Warn("Component workflow run does not belong to the specified component")
		return nil, ErrComponentWorkflowRunNotFound
	}

	// Extract overall status of the workflow run from conditions
	overallStatus := getComponentWorkflowStatus(workflowRun.Status.Conditions)

	// Map tasks to WorkflowStepStatus
	steps := make([]models.WorkflowStepStatus, 0, len(workflowRun.Status.Tasks))
	for _, task := range workflowRun.Status.Tasks {
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

	// Determine if the workflow run has live observability by checking whether the
	// Argo Workflow resource still exists on the build plane. If it exists, logs/events
	// can be fetched live from the build plane; otherwise, they are not available live.
	hasLiveObservability := s.checkArgoWorkflowExistsOnBuildPlane(ctx, namespaceName, gatewayURL, workflowRun.Status.RunReference)

	return &models.ComponentWorkflowRunStatusResponse{
		Status:               overallStatus,
		Steps:                steps,
		HasLiveObservability: hasLiveObservability,
	}, nil
}

// checkArgoWorkflowExistsOnBuildPlane checks whether the Argo Workflow resource
// referenced by the given RunReference still exists on the build plane.
// Returns true if the workflow exists, false otherwise
func (s *ComponentWorkflowService) checkArgoWorkflowExistsOnBuildPlane(
	ctx context.Context,
	namespaceName string,
	gatewayURL string,
	runReference *openchoreov1alpha1.ResourceReference,
) bool {
	if runReference == nil || runReference.Name == "" || runReference.Namespace == "" {
		return false
	}

	bpClient, err := s.buildPlaneService.GetBuildPlaneClient(ctx, namespaceName, gatewayURL)
	if err != nil {
		s.logger.Debug("Failed to get build plane client for workflow existence check", "error", err)
		return false
	}

	var argoWorkflow argoproj.Workflow
	if err := bpClient.Get(ctx, types.NamespacedName{
		Name:      runReference.Name,
		Namespace: runReference.Namespace,
	}, &argoWorkflow); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false
		}
		s.logger.Debug("Failed to check argo workflow existence on build plane", "error", err)
		return false
	}

	return true
}

// GetComponentWorkflowRunLogs retrieves logs from a component workflow run
func (s *ComponentWorkflowService) GetComponentWorkflowRunLogs(ctx context.Context, namespaceName, projectName, componentName, runName, stepName, gatewayURL string, sinceSeconds *int64) ([]models.ComponentWorkflowRunLogEntry, error) {
	logger := s.logger.With("namespace", namespaceName, "project", projectName, "component", componentName, "run", runName, "step", stepName, "sinceSeconds", sinceSeconds)
	logger.Debug("Getting component workflow run logs")

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponentWorkflowRun, ResourceTypeComponentWorkflowRun, runName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	// Get ComponentWorkflowRun
	var workflowRun openchoreov1alpha1.ComponentWorkflowRun
	err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      runName,
		Namespace: namespaceName,
	}, &workflowRun)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Warn("Component workflow run not found")
			return nil, ErrComponentWorkflowRunNotFound
		}
		logger.Error("Failed to get component workflow run", "error", err)
		return nil, fmt.Errorf("failed to get component workflow run: %w", err)
	}

	// Verify the workflow run belongs to the specified component
	if workflowRun.Spec.Owner.ProjectName != projectName || workflowRun.Spec.Owner.ComponentName != componentName {
		logger.Warn("Component workflow run does not belong to the specified component")
		return nil, ErrComponentWorkflowRunNotFound
	}

	// Check if RunReference exists
	if workflowRun.Status.RunReference == nil || workflowRun.Status.RunReference.Name == "" || workflowRun.Status.RunReference.Namespace == "" {
		logger.Warn("Workflow run reference not found")
		return nil, fmt.Errorf("workflow run reference not found")
	}

	// Get logs through the build plane client (Argo specific)
	// TODO: Extend to support other build engines (eg. Jenkins)
	return s.getArgoWorkflowRunLogs(ctx, namespaceName, gatewayURL, workflowRun.Status.RunReference, stepName, sinceSeconds)
}

// getArgoWorkflowRunLogs retrieves logs from an Argo Workflow run
// This function handles Argo-specific logic for finding workflow pods and retrieving logs
func (s *ComponentWorkflowService) getArgoWorkflowRunLogs(
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
			logger.Warn("Argo workflow not found in build plane", "workflow", runReference.Name, "namespace", runReference.Namespace)
			return nil, fmt.Errorf("argo workflow not found")
		}
		logger.Error("Failed to get argo workflow", "error", err)
		return nil, fmt.Errorf("failed to get argo workflow: %w", err)
	}

	// Get pods for the workflow/step
	pods, err := s.getArgoWorkflowPods(ctx, bpClient, &argoWorkflow, stepName)
	if err != nil {
		return nil, fmt.Errorf("failed to get Argo workflow pods: %w", err)
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
		// Kubernetes API with timestamps=true returns logs in format: "2024-01-10T12:34:56.789Z log message"
		lines := strings.Split(podLogs, "\n")
		for _, line := range lines {
			trimmedLine := strings.TrimSpace(line)
			if trimmedLine == "" || trimmedLine == "---" {
				continue
			}

			// Extract timestamp and log message
			// Timestamp is at the beginning, followed by a space, then the log message
			spaceIndex := strings.Index(trimmedLine, " ")
			if spaceIndex > 0 {
				timestampCandidate := trimmedLine[:spaceIndex]
				// Try to parse as RFC3339 or RFC3339Nano timestamp
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

			// No valid timestamp found, use empty timestamp
			allLogEntries = append(allLogEntries, models.ComponentWorkflowRunLogEntry{
				Timestamp: "",
				Log:       trimmedLine,
			})
		}
	}

	return allLogEntries, nil
}

// getArgoWorkflowPods finds pods for a workflow and optionally a specific step
func (s *ComponentWorkflowService) getArgoWorkflowPods(ctx context.Context, bpClient client.Client, workflow *argoproj.Workflow, stepName string) ([]corev1.Pod, error) {
	// Build label selector for workflow pods
	selector := labels.Set{
		"workflows.argoproj.io/workflow": workflow.Name,
	}

	var podList corev1.PodList
	err := bpClient.List(ctx, &podList, client.InNamespace(workflow.Namespace), client.MatchingLabelsSelector{Selector: selector.AsSelector()})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	if len(podList.Items) == 0 {
		return nil, fmt.Errorf("no pods found for workflow")
	}

	// If stepName is specified, filter by step
	if stepName != "" {
		filteredPods := make([]corev1.Pod, 0)
		for _, pod := range podList.Items {
			if strings.Contains(pod.Name, stepName) {
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

// getArgoWorkflowPodLogs retrieves logs from an Argo Workflow pod using the gateway client
// Fetches logs from all containers excluding Argo sidecar containers (init and wait)
func (s *ComponentWorkflowService) getArgoWorkflowPodLogs(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane, pod *corev1.Pod, sinceSeconds *int64) (string, error) {
	if s.gwClient == nil {
		return "", fmt.Errorf("gateway client is not configured")
	}

	excludedContainersForLogs := map[string]bool{
		"wait": true,
		"init": true,
	}

	// Get container names from pod spec, excluding Argo sidecar containers
	containerNames := make([]string, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		if !excludedContainersForLogs[container.Name] {
			containerNames = append(containerNames, container.Name)
		}
	}
	if len(containerNames) == 0 {
		return "", fmt.Errorf("no containers to fetch logs from in pod")
	}

	// Fetch logs from all containers and merge them
	var allLogs strings.Builder
	for i, containerName := range containerNames {
		if i > 0 {
			allLogs.WriteString("\n---\n")
		}

		// Use gateway client to get pod logs
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

// GetComponentWorkflowRunEvents retrieves events from a component workflow run
func (s *ComponentWorkflowService) GetComponentWorkflowRunEvents(ctx context.Context, namespaceName, projectName, componentName, runName, stepName, gatewayURL string) ([]models.ComponentWorkflowRunEventEntry, error) {
	logger := s.logger.With("namespace", namespaceName, "project", projectName, "component", componentName, "run", runName, "step", stepName)
	logger.Debug("Getting component workflow run events")

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponentWorkflowRun, ResourceTypeComponentWorkflowRun, runName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	// Get ComponentWorkflowRun
	var workflowRun openchoreov1alpha1.ComponentWorkflowRun
	if err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      runName,
		Namespace: namespaceName,
	}, &workflowRun); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Warn("Component workflow run not found", "namespace", namespaceName, "run", runName)
			return nil, ErrComponentWorkflowRunNotFound
		}
		logger.Error("Failed to get component workflow run", "error", err)
		return nil, fmt.Errorf("failed to get component workflow run: %w", err)
	}

	// Verify the workflow run belongs to the specified component
	if workflowRun.Spec.Owner.ProjectName != projectName || workflowRun.Spec.Owner.ComponentName != componentName {
		logger.Warn("Component workflow run does not belong to the specified component",
			"namespace", namespaceName, "project", projectName, "component", componentName, "run", runName)
		return nil, ErrComponentWorkflowRunNotFound
	}

	// Check if RunReference exists
	if workflowRun.Status.RunReference == nil || workflowRun.Status.RunReference.Name == "" || workflowRun.Status.RunReference.Namespace == "" {
		logger.Warn("Workflow run reference not found", "run", runName)
		return nil, fmt.Errorf("workflow run reference not found")
	}

	// Get events through the build plane client (Argo specific)
	// TODO: Extend to support other build engines (eg. Jenkins)
	return s.getArgoWorkflowRunEvents(ctx, namespaceName, gatewayURL, workflowRun.Status.RunReference, stepName)
}

// getArgoWorkflowRunEvents retrieves events from an Argo Workflow run
// This function handles Argo-specific logic for finding workflow pods and retrieving events
func (s *ComponentWorkflowService) getArgoWorkflowRunEvents(
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
			return nil, fmt.Errorf("argo workflow not found")
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

		// Convert Kubernetes events to ComponentWorkflowRunEventEntry objects
		for _, event := range podEvents.Items {
			// Use EventTime if available, otherwise use FirstTimestamp
			var timestamp time.Time
			if !event.EventTime.IsZero() {
				timestamp = event.EventTime.Time
			} else if !event.FirstTimestamp.IsZero() {
				timestamp = event.FirstTimestamp.Time
			} else {
				// Use current time if neither is available
				timestamp = time.Now()
			}

			eventEntry := models.ComponentWorkflowRunEventEntry{
				Timestamp: timestamp.Format(time.RFC3339),
				Type:      event.Type,
				Reason:    event.Reason,
				Message:   event.Message,
			}
			allEventEntries = append(allEventEntries, eventEntry)
		}
	}

	return allEventEntries, nil
}

// getPodEvents retrieves events for a pod using the gateway client
func (s *ComponentWorkflowService) getArgoWorkflowPodEvents(ctx context.Context, buildPlane *openchoreov1alpha1.BuildPlane, pod *corev1.Pod) (*corev1.EventList, error) {
	if s.gwClient == nil {
		return nil, fmt.Errorf("gateway client is not configured")
	}

	// Use gateway client to get pod events
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
