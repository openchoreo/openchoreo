// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/schema"
)

// ComponentWorkflowService handles component workflow-related business logic
type ComponentWorkflowService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

// NewComponentWorkflowService creates a new component workflow service
func NewComponentWorkflowService(k8sClient client.Client, logger *slog.Logger) *ComponentWorkflowService {
	return &ComponentWorkflowService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// TriggerWorkflow creates a new ComponentWorkflowRun from a component's workflow configuration
func (s *ComponentWorkflowService) TriggerWorkflow(ctx context.Context, orgName, projectName, componentName, commit string) (*models.ComponentWorkflowResponse, error) {
	s.logger.Debug("Triggering component workflow", "org", orgName, "project", projectName, "component", componentName, "commit", commit)

	// Retrieve component and use that to create the workflow run
	var component openchoreov1alpha1.Component
	err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      componentName,
		Namespace: orgName,
	}, &component)

	if err != nil {
		s.logger.Error("Failed to get component", "error", err, "org", orgName, "project", projectName, "component", componentName)
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
	systemParams.Repository.Revision.Commit = commit

	// Generate a unique workflow run name with short UUID
	uuid, err := generateShortUUID()
	if err != nil {
		s.logger.Error("Failed to generate UUID", "error", err)
		return nil, fmt.Errorf("failed to generate UUID: %w", err)
	}
	workflowRunName := fmt.Sprintf("%s-%s", componentName, uuid)

	// Create the ComponentWorkflowRun CR
	workflowRun := &openchoreov1alpha1.ComponentWorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workflowRunName,
			Namespace: orgName,
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
		OrgName:       orgName,
		Commit:        commit,
		Status:        "Pending",
		CreatedAt:     workflowRun.CreationTimestamp.Time,
		Image:         "",
	}, nil
}

// ListComponentWorkflowRuns retrieves component workflow runs for a component using spec.owner fields
func (s *ComponentWorkflowService) ListComponentWorkflowRuns(ctx context.Context, orgName, projectName, componentName string) ([]models.ComponentWorkflowResponse, error) {
	s.logger.Debug("Listing component workflow runs", "org", orgName, "project", projectName, "component", componentName)

	var workflowRuns openchoreov1alpha1.ComponentWorkflowRunList
	err := s.k8sClient.List(ctx, &workflowRuns, client.InNamespace(orgName))
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
			OrgName:       orgName,
			Commit:        commit,
			Status:        getComponentWorkflowStatus(workflowRun.Status.Conditions),
			CreatedAt:     workflowRun.CreationTimestamp.Time,
			Image:         workflowRun.Status.ImageStatus.Image,
		})
	}

	return workflowResponses, nil
}

// getComponentWorkflowStatus determines the user-friendly status from component workflow run conditions
func getComponentWorkflowStatus(workflowConditions []metav1.Condition) string {
	if len(workflowConditions) == 0 {
		return "Pending"
	}

	// Check conditions in priority order
	// Similar to build workflow status logic
	for _, condition := range workflowConditions {
		if condition.Type == "WorkloadUpdated" && condition.Status == metav1.ConditionTrue {
			return "Completed"
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

	return "Pending"
}

// ListComponentWorkflows lists all ComponentWorkflow templates in the given organization
func (s *ComponentWorkflowService) ListComponentWorkflows(ctx context.Context, orgName string) ([]*models.WorkflowResponse, error) {
	s.logger.Debug("Listing ComponentWorkflow templates", "org", orgName)

	var cwfList openchoreov1alpha1.ComponentWorkflowList
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, &cwfList, listOpts...); err != nil {
		s.logger.Error("Failed to list ComponentWorkflow templates", "error", err)
		return nil, fmt.Errorf("failed to list ComponentWorkflow templates: %w", err)
	}

	cwfs := make([]*models.WorkflowResponse, 0, len(cwfList.Items))
	for i := range cwfList.Items {
		cwfs = append(cwfs, s.toComponentWorkflowResponse(&cwfList.Items[i]))
	}

	s.logger.Debug("Listed ComponentWorkflow templates", "org", orgName, "count", len(cwfs))
	return cwfs, nil
}

// GetComponentWorkflow retrieves a specific ComponentWorkflow template
func (s *ComponentWorkflowService) GetComponentWorkflow(ctx context.Context, orgName, cwfName string) (*models.WorkflowResponse, error) {
	s.logger.Debug("Getting ComponentWorkflow", "org", orgName, "name", cwfName)

	cwf := &openchoreov1alpha1.ComponentWorkflow{}
	key := client.ObjectKey{
		Name:      cwfName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, cwf); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ComponentWorkflow template not found", "org", orgName, "name", cwfName)
			return nil, ErrComponentWorkflowNotFound
		}
		s.logger.Error("Failed to get ComponentWorkflow template", "error", err)
		return nil, fmt.Errorf("failed to get ComponentWorkflow template: %w", err)
	}

	return s.toComponentWorkflowResponse(cwf), nil
}

// GetComponentWorkflowSchema retrieves the JSON schema for a ComponentWorkflow template
func (s *ComponentWorkflowService) GetComponentWorkflowSchema(ctx context.Context, orgName, cwfName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting ComponentWorkflow template schema", "org", orgName, "name", cwfName)

	cwf := &openchoreov1alpha1.ComponentWorkflow{}
	key := client.ObjectKey{
		Name:      cwfName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, cwf); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ComponentWorkflow template not found", "org", orgName, "name", cwfName)
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
		schemaMap["systemParameters"] = map[string]any{
			"repository": map[string]any{
				"url": cwf.Spec.Schema.SystemParameters.Repository.URL,
				"revision": map[string]any{
					"branch": cwf.Spec.Schema.SystemParameters.Repository.Revision.Branch,
					"commit": cwf.Spec.Schema.SystemParameters.Repository.Revision.Commit,
				},
				"appPath": cwf.Spec.Schema.SystemParameters.Repository.AppPath,
			},
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
	}

	jsonSchema, err := schema.ToJSONSchema(def)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved ComponentWorkflow template schema successfully", "org", orgName, "name", cwfName)
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
