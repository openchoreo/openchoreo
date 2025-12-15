// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"
	"log/slog"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/schema/extractor"
)

// WorkflowService handles Workflow-related business logic
type WorkflowService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

// NewWorkflowService creates a new Workflow service
func NewWorkflowService(k8sClient client.Client, logger *slog.Logger) *WorkflowService {
	return &WorkflowService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// ListWorkflows lists all Workflows in the given organization
func (s *WorkflowService) ListWorkflows(ctx context.Context, orgName string) ([]*models.WorkflowResponse, error) {
	s.logger.Debug("Listing Workflows", "org", orgName)

	var wfList openchoreov1alpha1.WorkflowList
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, &wfList, listOpts...); err != nil {
		s.logger.Error("Failed to list Workflows", "error", err)
		return nil, fmt.Errorf("failed to list Workflows: %w", err)
	}

	wfs := make([]*models.WorkflowResponse, 0, len(wfList.Items))
	for i := range wfList.Items {
		wfs = append(wfs, s.toWorkflowResponse(&wfList.Items[i]))
	}

	s.logger.Debug("Listed Workflows", "org", orgName, "count", len(wfs))
	return wfs, nil
}

// GetWorkflow retrieves a specific Workflow
func (s *WorkflowService) GetWorkflow(ctx context.Context, orgName, wfName string) (*models.WorkflowResponse, error) {
	s.logger.Debug("Getting Workflow", "org", orgName, "name", wfName)

	wf := &openchoreov1alpha1.Workflow{}
	key := client.ObjectKey{
		Name:      wfName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, wf); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workflow not found", "org", orgName, "name", wfName)
			return nil, ErrWorkflowNotFound
		}
		s.logger.Error("Failed to get Workflow", "error", err)
		return nil, fmt.Errorf("failed to get Workflow: %w", err)
	}

	return s.toWorkflowResponse(wf), nil
}

// GetWorkflowSchema retrieves the JSON schema for a Workflow
func (s *WorkflowService) GetWorkflowSchema(ctx context.Context, orgName, wfName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting Workflow schema", "org", orgName, "name", wfName)

	wf := &openchoreov1alpha1.Workflow{}
	key := client.ObjectKey{
		Name:      wfName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, wf); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workflow not found", "org", orgName, "name", wfName)
			return nil, ErrWorkflowNotFound
		}
		s.logger.Error("Failed to get Workflow", "error", err)
		return nil, fmt.Errorf("failed to get Workflow: %w", err)
	}

	var schemaMap map[string]any
	if wf.Spec.Schema != nil && wf.Spec.Schema.Raw != nil {
		if err := yaml.Unmarshal(wf.Spec.Schema.Raw, &schemaMap); err != nil {
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

	s.logger.Debug("Retrieved Workflow schema successfully", "org", orgName, "name", wfName)
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
