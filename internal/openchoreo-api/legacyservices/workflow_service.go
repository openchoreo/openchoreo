// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/schema/extractor"
)

// WorkflowService handles Workflow-related business logic
type WorkflowService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewWorkflowService creates a new Workflow service
func NewWorkflowService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *WorkflowService {
	return &WorkflowService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// AuthorizeCreate checks if the current user is authorized to create a Workflow
func (s *WorkflowService) AuthorizeCreate(ctx context.Context, namespaceName, wfName string) error {
	return checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateWorkflow,
		ResourceTypeWorkflow, wfName, authz.ResourceHierarchy{Namespace: namespaceName})
}

// ListWorkflows lists all Workflows in the given namespace
func (s *WorkflowService) ListWorkflows(ctx context.Context, namespaceName string) ([]*models.WorkflowResponse, error) {
	s.logger.Debug("Listing Workflows", "namespace", namespaceName)

	var wfList openchoreov1alpha1.WorkflowList
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}

	if err := s.k8sClient.List(ctx, &wfList, listOpts...); err != nil {
		s.logger.Error("Failed to list Workflows", "error", err)
		return nil, fmt.Errorf("failed to list Workflows: %w", err)
	}

	wfs := make([]*models.WorkflowResponse, 0, len(wfList.Items))
	for i := range wfList.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewWorkflow, ResourceTypeWorkflow, wfList.Items[i].Name,
			authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				// Skip unauthorized items
				s.logger.Debug("Skipping unauthorized workflow", "namespace", namespaceName, "workflow", wfList.Items[i].Name)
				continue
			}
			return nil, err
		}
		wfs = append(wfs, s.toWorkflowResponse(&wfList.Items[i]))
	}

	s.logger.Debug("Listed Workflows", "namespace", namespaceName, "count", len(wfs))
	return wfs, nil
}

// GetWorkflow retrieves a specific Workflow
func (s *WorkflowService) GetWorkflow(ctx context.Context, namespaceName, wfName string) (*models.WorkflowResponse, error) {
	s.logger.Debug("Getting Workflow", "namespace", namespaceName, "name", wfName)

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewWorkflow, ResourceTypeWorkflow, wfName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	wf := &openchoreov1alpha1.Workflow{}
	key := client.ObjectKey{
		Name:      wfName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, wf); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workflow not found", "namespace", namespaceName, "name", wfName)
			return nil, ErrWorkflowNotFound
		}
		s.logger.Error("Failed to get Workflow", "error", err)
		return nil, fmt.Errorf("failed to get Workflow: %w", err)
	}

	return s.toWorkflowResponse(wf), nil
}

// GetWorkflowSchema retrieves the JSON schema for a Workflow
func (s *WorkflowService) GetWorkflowSchema(ctx context.Context, namespaceName, wfName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting Workflow schema", "namespace", namespaceName, "name", wfName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewWorkflow, ResourceTypeWorkflow, wfName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	wf := &openchoreov1alpha1.Workflow{}
	key := client.ObjectKey{
		Name:      wfName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, wf); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workflow not found", "namespace", namespaceName, "name", wfName)
			return nil, ErrWorkflowNotFound
		}
		s.logger.Error("Failed to get Workflow", "error", err)
		return nil, fmt.Errorf("failed to get Workflow: %w", err)
	}

	var schemaMap map[string]any
	if wf.Spec.Schema != nil && wf.Spec.Schema.Parameters != nil {
		if err := yaml.Unmarshal(wf.Spec.Schema.Parameters.Raw, &schemaMap); err != nil {
			return nil, fmt.Errorf("failed to extract schema: %w", err)
		}
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

	s.logger.Debug("Retrieved Workflow schema successfully", "namespace", namespaceName, "name", wfName)
	return jsonSchema, nil
}

func (s *WorkflowService) toWorkflowResponse(wf *openchoreov1alpha1.Workflow) *models.WorkflowResponse {
	return &models.WorkflowResponse{
		Name:        wf.Name,
		DisplayName: wf.Annotations[controller.AnnotationKeyDisplayName],
		Description: wf.Annotations[controller.AnnotationKeyDescription],
		CreatedAt:   wf.CreationTimestamp.Time,
	}
}
