// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"log/slog"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionViewWorkflow   = "workflow:view"
	actionCreateWorkflow = "workflow:create"
	actionUpdateWorkflow = "workflow:update"
	actionDeleteWorkflow = "workflow:delete"

	resourceTypeWorkflow = "workflow"
)

// workflowServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type workflowServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*workflowServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a workflow service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &workflowServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *workflowServiceWithAuthz) ListWorkflows(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Workflow], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Workflow], error) {
			return s.internal.ListWorkflows(ctx, namespaceName, pageOpts)
		},
		func(wf openchoreov1alpha1.Workflow) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewWorkflow,
				ResourceType: resourceTypeWorkflow,
				ResourceID:   wf.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *workflowServiceWithAuthz) GetWorkflow(ctx context.Context, namespaceName, workflowName string) (*openchoreov1alpha1.Workflow, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewWorkflow,
		ResourceType: resourceTypeWorkflow,
		ResourceID:   workflowName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetWorkflow(ctx, namespaceName, workflowName)
}

func (s *workflowServiceWithAuthz) GetWorkflowSchema(ctx context.Context, namespaceName, workflowName string) (*extv1.JSONSchemaProps, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewWorkflow,
		ResourceType: resourceTypeWorkflow,
		ResourceID:   workflowName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetWorkflowSchema(ctx, namespaceName, workflowName)
}

func (s *workflowServiceWithAuthz) CreateWorkflow(ctx context.Context, namespaceName string, wf *openchoreov1alpha1.Workflow) (*openchoreov1alpha1.Workflow, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateWorkflow,
		ResourceType: resourceTypeWorkflow,
		ResourceID:   wf.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateWorkflow(ctx, namespaceName, wf)
}

func (s *workflowServiceWithAuthz) UpdateWorkflow(ctx context.Context, namespaceName string, wf *openchoreov1alpha1.Workflow) (*openchoreov1alpha1.Workflow, error) {
	if wf == nil {
		return nil, ErrWorkflowNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateWorkflow,
		ResourceType: resourceTypeWorkflow,
		ResourceID:   wf.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateWorkflow(ctx, namespaceName, wf)
}

func (s *workflowServiceWithAuthz) DeleteWorkflow(ctx context.Context, namespaceName, workflowName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteWorkflow,
		ResourceType: resourceTypeWorkflow,
		ResourceID:   workflowName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteWorkflow(ctx, namespaceName, workflowName)
}
