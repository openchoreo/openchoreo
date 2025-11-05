// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"golang.org/x/exp/slog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	argo "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// BuildService handles build-related business logic
type BuildService struct {
	k8sClient         client.Client
	logger            *slog.Logger
	buildPlaneService *BuildPlaneService
	bpClientMgr       *kubernetesClient.KubeMultiClientManager
}

// NewBuildService creates a new build service
func NewBuildService(k8sClient client.Client, buildPlaneService *BuildPlaneService, bpClientMgr *kubernetesClient.KubeMultiClientManager, logger *slog.Logger) *BuildService {
	return &BuildService{
		k8sClient:         k8sClient,
		logger:            logger,
		buildPlaneService: buildPlaneService,
		bpClientMgr:       bpClientMgr,
	}
}

// ListBuildTemplates retrieves cluster workflow templates (argo) available for an organization in the buildplane
func (s *BuildService) ListBuildTemplates(ctx context.Context, orgName string) ([]models.BuildTemplateResponse, error) {
	s.logger.Debug("Listing build templates", "org", orgName)

	// Get the build plane Kubernetes client
	buildPlaneClient, err := s.buildPlaneService.GetBuildPlaneClient(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane client: %w", err)
	}

	// List ClusterWorkflowTemplates using the build plane client
	var clusterWorkflowTemplates argo.ClusterWorkflowTemplateList
	err = buildPlaneClient.List(ctx, &clusterWorkflowTemplates)
	if err != nil {
		s.logger.Error("Failed to list ClusterWorkflowTemplates", "error", err)
		return nil, fmt.Errorf("failed to list ClusterWorkflowTemplates: %w", err)
	}

	s.logger.Debug("Found build templates", "count", len(clusterWorkflowTemplates.Items), "org", orgName)

	templateResponses := make([]models.BuildTemplateResponse, 0, len(clusterWorkflowTemplates.Items))
	for _, template := range clusterWorkflowTemplates.Items {
		parameters := make([]models.BuildTemplateParameter, 0, len(template.Spec.Arguments.Parameters))
		if template.Spec.Arguments.Parameters != nil {
			for _, param := range template.Spec.Arguments.Parameters {
				templateParam := models.BuildTemplateParameter{
					Name: param.Name,
				}

				if param.Default != nil {
					templateParam.Default = string(*param.Default)
				}

				parameters = append(parameters, templateParam)
			}
		}

		templateResponse := models.BuildTemplateResponse{
			Name:       template.Name,
			Parameters: parameters,
			CreatedAt:  template.CreationTimestamp.Time,
		}

		templateResponses = append(templateResponses, templateResponse)
	}

	return templateResponses, nil
}

// TriggerBuild creates a new workflow from a component's build configuration
func (s *BuildService) TriggerBuild(ctx context.Context, orgName, projectName, componentName, commit string) (*models.BuildResponse, error) {
	s.logger.Debug("Triggering build", "org", orgName, "project", projectName, "component", componentName, "commit", commit)

	// Retrieve component and use that to create the workflow
	var component openchoreov1alpha1.Component
	err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      componentName,
		Namespace: orgName,
	}, &component)

	if err != nil {
		s.logger.Error("Failed to get component", "error", err, "org", orgName, "project", projectName, "component", componentName)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Check if component has build configuration
	if component.Spec.Build.WorkflowTemplate == "" {
		s.logger.Error("Component does not have a workflow template configured", "component", componentName)
		return nil, fmt.Errorf("component %s does not have a workflow template configured", componentName)
	}

	// Copy the schema from the component and update the commit
	var schemaMap map[string]interface{}
	if component.Spec.Build.Schema != nil && component.Spec.Build.Schema.Raw != nil {
		if err := json.Unmarshal(component.Spec.Build.Schema.Raw, &schemaMap); err != nil {
			s.logger.Error("Failed to unmarshal component schema", "error", err)
			return nil, fmt.Errorf("failed to unmarshal component schema: %w", err)
		}
	} else {
		schemaMap = make(map[string]interface{})
	}

	// Update the commit in the schema
	if repo, ok := schemaMap["repository"].(map[string]interface{}); ok {
		if revision, ok := repo["revision"].(map[string]interface{}); ok {
			revision["commit"] = commit
		} else {
			repo["revision"] = map[string]interface{}{
				"commit": commit,
			}
		}
	} else {
		schemaMap["repository"] = map[string]interface{}{
			"revision": map[string]interface{}{
				"commit": commit,
			},
		}
	}

	// Marshal the updated schema back to JSON
	updatedSchemaBytes, err := json.Marshal(schemaMap)
	if err != nil {
		s.logger.Error("Failed to marshal updated schema", "error", err)
		return nil, fmt.Errorf("failed to marshal updated schema: %w", err)
	}

	// Generate a unique workflow name with short UUID
	uuid, err := generateShortUUID()
	if err != nil {
		s.logger.Error("Failed to generate UUID", "error", err)
		return nil, fmt.Errorf("failed to generate UUID: %w", err)
	}
	workflowName := fmt.Sprintf("%s-%s", componentName, uuid)

	// Create the Workflow CR
	workflow := &openchoreov1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workflowName,
			Namespace: orgName,
		},
		Spec: openchoreov1alpha1.WorkflowSpec{
			Owner: openchoreov1alpha1.WorkflowOwner{
				ProjectName:   projectName,
				ComponentName: componentName,
			},
			WorkflowDefinitionRef: component.Spec.Build.WorkflowTemplate,
			Schema: &runtime.RawExtension{
				Raw: updatedSchemaBytes,
			},
		},
	}

	if err := s.k8sClient.Create(ctx, workflow); err != nil {
		s.logger.Error("Failed to create workflow", "error", err)
		return nil, fmt.Errorf("failed to create workflow: %w", err)
	}

	s.logger.Info("Workflow created successfully", "workflow", workflowName, "component", componentName, "commit", commit)

	// Return a BuildResponse for API compatibility
	return &models.BuildResponse{
		Name:          workflow.Name,
		UUID:          string(workflow.UID),
		ComponentName: componentName,
		ProjectName:   projectName,
		OrgName:       orgName,
		Commit:        commit,
		Status:        "Pending",
		CreatedAt:     workflow.CreationTimestamp.Time,
		Image:         "",
	}, nil
}

// ListBuilds retrieves workflows for a component using spec.owner fields
func (s *BuildService) ListBuilds(ctx context.Context, orgName, projectName, componentName string) ([]models.BuildResponse, error) {
	s.logger.Debug("Listing builds", "org", orgName, "project", projectName, "component", componentName)

	var workflows openchoreov1alpha1.WorkflowList
	err := s.k8sClient.List(ctx, &workflows, client.InNamespace(orgName))
	if err != nil {
		s.logger.Error("Failed to list workflows", "error", err)
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}

	buildResponses := make([]models.BuildResponse, 0, len(workflows.Items))
	for _, workflow := range workflows.Items {
		// Filter by spec.owner fields
		if workflow.Spec.Owner.ProjectName != projectName || workflow.Spec.Owner.ComponentName != componentName {
			continue
		}

		// Extract commit from the workflow schema
		commit := extractCommitFromSchema(workflow.Spec.Schema)
		if commit == "" {
			commit = "latest"
		}

		buildResponses = append(buildResponses, models.BuildResponse{
			Name:          workflow.Name,
			UUID:          string(workflow.UID),
			ComponentName: componentName,
			ProjectName:   projectName,
			OrgName:       orgName,
			Commit:        commit,
			Status:        GetLatestWorkflowStatus(workflow.Status.Conditions),
			CreatedAt:     workflow.CreationTimestamp.Time,
			Image:         workflow.Status.ImageStatus.Image,
		})
	}

	return buildResponses, nil
}

// extractCommitFromSchema extracts the commit hash from the workflow schema
func extractCommitFromSchema(schema *runtime.RawExtension) string {
	if schema == nil || schema.Raw == nil {
		return ""
	}

	var schemaMap map[string]interface{}
	if err := json.Unmarshal(schema.Raw, &schemaMap); err != nil {
		return ""
	}

	// Navigate to repository.revision.commit
	if repo, ok := schemaMap["repository"].(map[string]interface{}); ok {
		if revision, ok := repo["revision"].(map[string]interface{}); ok {
			if commit, ok := revision["commit"].(string); ok {
				return commit
			}
		}
	}

	return ""
}

// GetLatestWorkflowStatus determines the user-friendly status from workflow conditions
func GetLatestWorkflowStatus(workflowConditions []metav1.Condition) string {
	if len(workflowConditions) == 0 {
		return "Pending"
	}

	// Check conditions in priority order
	// WorkloadUpdated > WorkflowCompleted > WorkflowRunning
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

// generateShortUUID generates a short 8-character UUID for workflow naming.
func generateShortUUID() (string, error) {
	bytes := make([]byte, 4) // 4 bytes = 8 hex characters
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
