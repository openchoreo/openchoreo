// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowplane

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeWorkflowPlane = "workflowplane"
)

// workflowPlaneServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type workflowPlaneServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*workflowPlaneServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a workflow plane service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &workflowPlaneServiceWithAuthz{
		internal: NewService(k8sClient, nil, logger), // nil provider is OK — authz wrapper stubs GetWorkflowPlaneClient/ArgoWorkflowExists
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *workflowPlaneServiceWithAuthz) ListWorkflowPlanes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.WorkflowPlane], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.WorkflowPlane], error) {
			return s.internal.ListWorkflowPlanes(ctx, namespaceName, pageOpts)
		},
		func(wp openchoreov1alpha1.WorkflowPlane) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewWorkflowPlane,
				ResourceType: resourceTypeWorkflowPlane,
				ResourceID:   wp.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *workflowPlaneServiceWithAuthz) GetWorkflowPlane(ctx context.Context, namespaceName, workflowPlaneName string) (*openchoreov1alpha1.WorkflowPlane, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewWorkflowPlane,
		ResourceType: resourceTypeWorkflowPlane,
		ResourceID:   workflowPlaneName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetWorkflowPlane(ctx, namespaceName, workflowPlaneName)
}

// CreateWorkflowPlane checks create authorization before delegating to the internal service.
func (s *workflowPlaneServiceWithAuthz) CreateWorkflowPlane(ctx context.Context, namespaceName string, wp *openchoreov1alpha1.WorkflowPlane) (*openchoreov1alpha1.WorkflowPlane, error) {
	if wp == nil {
		return nil, ErrWorkflowPlaneNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateWorkflowPlane,
		ResourceType: resourceTypeWorkflowPlane,
		ResourceID:   wp.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateWorkflowPlane(ctx, namespaceName, wp)
}

// UpdateWorkflowPlane checks update authorization before delegating to the internal service.
func (s *workflowPlaneServiceWithAuthz) UpdateWorkflowPlane(ctx context.Context, namespaceName string, wp *openchoreov1alpha1.WorkflowPlane) (*openchoreov1alpha1.WorkflowPlane, error) {
	if wp == nil {
		return nil, ErrWorkflowPlaneNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateWorkflowPlane,
		ResourceType: resourceTypeWorkflowPlane,
		ResourceID:   wp.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateWorkflowPlane(ctx, namespaceName, wp)
}

// DeleteWorkflowPlane checks delete authorization before delegating to the internal service.
func (s *workflowPlaneServiceWithAuthz) DeleteWorkflowPlane(ctx context.Context, namespaceName, workflowPlaneName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteWorkflowPlane,
		ResourceType: resourceTypeWorkflowPlane,
		ResourceID:   workflowPlaneName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteWorkflowPlane(ctx, namespaceName, workflowPlaneName)
}

// GetWorkflowPlaneClient is not implemented on the authz-wrapped service as it is not exposed externally.
func (s *workflowPlaneServiceWithAuthz) GetWorkflowPlaneClient(_ context.Context, _ string) (client.Client, error) {
	return nil, errNotImplemented
}

// ArgoWorkflowExists is not implemented on the authz-wrapped service as it is not exposed externally.
func (s *workflowPlaneServiceWithAuthz) ArgoWorkflowExists(_ context.Context, _ string, _ *openchoreov1alpha1.ResourceReference) bool {
	return false
}
