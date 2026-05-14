// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/schema"
)

var workflowTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "Workflow",
}

// workflowService handles workflow-related business logic without authorization checks.
type workflowService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*workflowService)(nil)

// NewService creates a new workflow service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &workflowService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *workflowService) ListWorkflows(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Workflow], error) {
	s.logger.Debug("Listing workflows", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	commonOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}
	listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

	var wfList openchoreov1alpha1.WorkflowList
	if err := s.k8sClient.List(ctx, &wfList, listOpts...); err != nil {
		s.logger.Error("Failed to list workflows", "error", err)
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}

	for i := range wfList.Items {
		wfList.Items[i].TypeMeta = workflowTypeMeta
	}

	result := &services.ListResult[openchoreov1alpha1.Workflow]{
		Items:      wfList.Items,
		NextCursor: wfList.Continue,
	}
	if wfList.RemainingItemCount != nil {
		remaining := *wfList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed workflows", "namespace", namespaceName, "count", len(wfList.Items))
	return result, nil
}

func (s *workflowService) GetWorkflow(ctx context.Context, namespaceName, workflowName string) (*openchoreov1alpha1.Workflow, error) {
	s.logger.Debug("Getting workflow", "namespace", namespaceName, "workflow", workflowName)

	wf := &openchoreov1alpha1.Workflow{}
	key := client.ObjectKey{
		Name:      workflowName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, wf); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workflow not found", "namespace", namespaceName, "workflow", workflowName)
			return nil, ErrWorkflowNotFound
		}
		s.logger.Error("Failed to get workflow", "error", err)
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	wf.TypeMeta = workflowTypeMeta
	return wf, nil
}

func (s *workflowService) GetWorkflowSchema(ctx context.Context, namespaceName, workflowName string) (map[string]any, error) {
	s.logger.Debug("Getting workflow schema", "namespace", namespaceName, "workflow", workflowName)

	wf, err := s.GetWorkflow(ctx, namespaceName, workflowName)
	if err != nil {
		return nil, err
	}

	// Convert to raw JSON Schema map, preserving vendor extensions (x-*) for frontend consumers
	rawSchema, err := schema.SectionToRawJSONSchema(wf.Spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved workflow schema successfully", "namespace", namespaceName, "workflow", workflowName)
	return rawSchema, nil
}

func (s *workflowService) CreateWorkflow(ctx context.Context, namespaceName string, wf *openchoreov1alpha1.Workflow) (*openchoreov1alpha1.Workflow, error) {
	if wf == nil {
		return nil, ErrWorkflowNil
	}

	s.logger.Debug("Creating workflow", "namespace", namespaceName, "workflow", wf.Name)

	// Check if workflow already exists
	existing := &openchoreov1alpha1.Workflow{}
	key := client.ObjectKey{
		Name:      wf.Name,
		Namespace: namespaceName,
	}
	if err := s.k8sClient.Get(ctx, key, existing); err == nil {
		return nil, ErrWorkflowAlreadyExists
	} else if client.IgnoreNotFound(err) != nil {
		return nil, fmt.Errorf("failed to check workflow existence: %w", err)
	}

	wf.Namespace = namespaceName
	wf.Status = openchoreov1alpha1.WorkflowStatus{}

	if err := s.k8sClient.Create(ctx, wf); err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to create workflow CR", "error", err)
		return nil, fmt.Errorf("failed to create workflow: %w", err)
	}

	wf.TypeMeta = workflowTypeMeta
	s.logger.Debug("Workflow created successfully", "namespace", namespaceName, "workflow", wf.Name)
	return wf, nil
}

func (s *workflowService) UpdateWorkflow(ctx context.Context, namespaceName string, wf *openchoreov1alpha1.Workflow) (*openchoreov1alpha1.Workflow, error) {
	if wf == nil {
		return nil, ErrWorkflowNil
	}

	s.logger.Debug("Updating workflow", "namespace", namespaceName, "workflow", wf.Name)

	existing := &openchoreov1alpha1.Workflow{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: wf.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrWorkflowNotFound
		}
		s.logger.Error("Failed to get workflow", "error", err)
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	wf.Status = openchoreov1alpha1.WorkflowStatus{}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = wf.Spec
	existing.Labels = wf.Labels
	existing.Annotations = wf.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to update workflow CR", "error", err)
		return nil, fmt.Errorf("failed to update workflow: %w", err)
	}

	existing.TypeMeta = workflowTypeMeta
	s.logger.Debug("Workflow updated successfully", "namespace", namespaceName, "workflow", wf.Name)
	return existing, nil
}

func (s *workflowService) DeleteWorkflow(ctx context.Context, namespaceName, workflowName string) error {
	s.logger.Debug("Deleting workflow", "namespace", namespaceName, "workflow", workflowName)

	wf := &openchoreov1alpha1.Workflow{}
	wf.Name = workflowName
	wf.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, wf); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrWorkflowNotFound
		}
		s.logger.Error("Failed to delete workflow CR", "error", err)
		return fmt.Errorf("failed to delete workflow: %w", err)
	}

	s.logger.Debug("Workflow deleted successfully", "namespace", namespaceName, "workflow", workflowName)
	return nil
}
