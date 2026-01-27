// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// WorkflowRunService handles WorkflowRun-related business logic
type WorkflowRunService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewWorkflowRunService creates a new WorkflowRun service
func NewWorkflowRunService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *WorkflowRunService {
	return &WorkflowRunService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// ListWorkflowRuns lists all WorkflowRuns in the given organization
func (s *WorkflowRunService) ListWorkflowRuns(ctx context.Context, orgName string) ([]*models.WorkflowRunResponse, error) {
	s.logger.Debug("Listing WorkflowRuns", "org", orgName)

	var wfRunList openchoreov1alpha1.WorkflowRunList
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, &wfRunList, listOpts...); err != nil {
		s.logger.Error("Failed to list WorkflowRuns", "error", err)
		return nil, fmt.Errorf("failed to list WorkflowRuns: %w", err)
	}

	wfRuns := make([]*models.WorkflowRunResponse, 0, len(wfRunList.Items))
	for i := range wfRunList.Items {
		// Authorization check
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewWorkflowRun, ResourceTypeWorkflowRun, wfRunList.Items[i].Name,
			authz.ResourceHierarchy{Namespace: orgName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				// Skip unauthorized items
				s.logger.Debug("Skipping unauthorized workflow run", "org", orgName, "workflowRun", wfRunList.Items[i].Name)
				continue
			}
			return nil, err
		}
		wfRuns = append(wfRuns, s.toWorkflowRunResponse(&wfRunList.Items[i]))
	}

	s.logger.Debug("Listed WorkflowRuns", "org", orgName, "count", len(wfRuns))
	return wfRuns, nil
}

// GetWorkflowRun retrieves a specific WorkflowRun
func (s *WorkflowRunService) GetWorkflowRun(ctx context.Context, orgName, runName string) (*models.WorkflowRunResponse, error) {
	s.logger.Debug("Getting WorkflowRun", "org", orgName, "run", runName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewWorkflowRun, ResourceTypeWorkflowRun, runName,
		authz.ResourceHierarchy{Namespace: orgName}); err != nil {
		return nil, err
	}

	wfRun := &openchoreov1alpha1.WorkflowRun{}
	key := client.ObjectKey{
		Name:      runName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, wfRun); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("WorkflowRun not found", "org", orgName, "run", runName)
			return nil, ErrWorkflowRunNotFound
		}
		s.logger.Error("Failed to get WorkflowRun", "error", err)
		return nil, fmt.Errorf("failed to get WorkflowRun: %w", err)
	}

	return s.toWorkflowRunResponse(wfRun), nil
}

// CreateWorkflowRun creates a new WorkflowRun
func (s *WorkflowRunService) CreateWorkflowRun(ctx context.Context, orgName string, req *models.CreateWorkflowRunRequest) (*models.WorkflowRunResponse, error) {
	s.logger.Debug("Creating WorkflowRun", "org", orgName, "workflow", req.WorkflowName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateWorkflowRun, ResourceTypeWorkflowRun, "",
		authz.ResourceHierarchy{Namespace: orgName}); err != nil {
		return nil, err
	}

	// Verify the referenced workflow exists
	workflow := &openchoreov1alpha1.Workflow{}
	workflowKey := client.ObjectKey{
		Name:      req.WorkflowName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, workflowKey, workflow); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Referenced workflow not found", "org", orgName, "workflow", req.WorkflowName)
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
			Namespace: orgName,
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
			s.logger.Warn("WorkflowRun already exists", "org", orgName, "run", runName)
			return nil, ErrWorkflowRunAlreadyExists
		}
		s.logger.Error("Failed to create WorkflowRun", "error", err)
		return nil, fmt.Errorf("failed to create WorkflowRun: %w", err)
	}

	s.logger.Debug("Created WorkflowRun successfully", "org", orgName, "run", runName, "workflow", req.WorkflowName)
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
		Name:         wfRun.Name,
		WorkflowName: wfRun.Spec.Workflow.Name,
		OrgName:      wfRun.Namespace,
		CreatedAt:    wfRun.CreationTimestamp.Time,
	}

	// Set UUID if available
	if wfRun.UID != "" {
		response.UUID = string(wfRun.UID)
	}

	// Extract status information
	if len(wfRun.Status.Conditions) > 0 {
		// Find the most recent condition that indicates the overall status
		for _, condition := range wfRun.Status.Conditions {
			if condition.Type == "Ready" || condition.Type == "Completed" {
				response.Status = string(condition.Status)
				response.Phase = condition.Reason
				if condition.LastTransitionTime.Time.After(wfRun.CreationTimestamp.Time) && condition.Status == metav1.ConditionFalse && condition.Type == "Completed" {
					response.FinishedAt = &condition.LastTransitionTime.Time
				}
				break
			}
		}
	}

	// Default status if not found
	if response.Status == "" {
		response.Status = "Pending"
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

// marshalToRawExtension marshals a map to runtime.RawExtension
func marshalToRawExtension(data map[string]interface{}) (*runtime.RawExtension, error) {
	if data == nil {
		return nil, nil
	}

	bytes, err := yaml.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data to YAML: %w", err)
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
	if err := yaml.Unmarshal(raw.Raw, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal raw extension: %w", err)
	}

	return result, nil
}