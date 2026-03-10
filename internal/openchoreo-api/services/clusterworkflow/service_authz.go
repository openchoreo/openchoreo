// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateClusterWorkflow = "clusterworkflow:create"
	actionUpdateClusterWorkflow = "clusterworkflow:update"
	actionDeleteClusterWorkflow = "clusterworkflow:delete"
	actionViewClusterWorkflow   = "clusterworkflow:view"

	resourceTypeClusterWorkflow = "clusterWorkflow"
)

// clusterWorkflowServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type clusterWorkflowServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*clusterWorkflowServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a cluster workflow service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &clusterWorkflowServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *clusterWorkflowServiceWithAuthz) CreateClusterWorkflow(ctx context.Context, cwf *openchoreov1alpha1.ClusterWorkflow) (*openchoreov1alpha1.ClusterWorkflow, error) {
	if cwf == nil {
		return nil, ErrClusterWorkflowNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateClusterWorkflow,
		ResourceType: resourceTypeClusterWorkflow,
		ResourceID:   cwf.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateClusterWorkflow(ctx, cwf)
}

func (s *clusterWorkflowServiceWithAuthz) UpdateClusterWorkflow(ctx context.Context, cwf *openchoreov1alpha1.ClusterWorkflow) (*openchoreov1alpha1.ClusterWorkflow, error) {
	if cwf == nil {
		return nil, ErrClusterWorkflowNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateClusterWorkflow,
		ResourceType: resourceTypeClusterWorkflow,
		ResourceID:   cwf.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateClusterWorkflow(ctx, cwf)
}

func (s *clusterWorkflowServiceWithAuthz) ListClusterWorkflows(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterWorkflow], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterWorkflow], error) {
			return s.internal.ListClusterWorkflows(ctx, pageOpts)
		},
		func(cwf openchoreov1alpha1.ClusterWorkflow) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewClusterWorkflow,
				ResourceType: resourceTypeClusterWorkflow,
				ResourceID:   cwf.Name,
				Hierarchy:    authz.ResourceHierarchy{},
			}
		},
	)
}

func (s *clusterWorkflowServiceWithAuthz) GetClusterWorkflow(ctx context.Context, clusterWorkflowName string) (*openchoreov1alpha1.ClusterWorkflow, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewClusterWorkflow,
		ResourceType: resourceTypeClusterWorkflow,
		ResourceID:   clusterWorkflowName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterWorkflow(ctx, clusterWorkflowName)
}

// DeleteClusterWorkflow checks delete authorization before delegating to the internal service.
func (s *clusterWorkflowServiceWithAuthz) DeleteClusterWorkflow(ctx context.Context, clusterWorkflowName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteClusterWorkflow,
		ResourceType: resourceTypeClusterWorkflow,
		ResourceID:   clusterWorkflowName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return err
	}
	return s.internal.DeleteClusterWorkflow(ctx, clusterWorkflowName)
}

func (s *clusterWorkflowServiceWithAuthz) GetClusterWorkflowSchema(ctx context.Context, clusterWorkflowName string) (map[string]any, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewClusterWorkflow,
		ResourceType: resourceTypeClusterWorkflow,
		ResourceID:   clusterWorkflowName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterWorkflowSchema(ctx, clusterWorkflowName)
}
