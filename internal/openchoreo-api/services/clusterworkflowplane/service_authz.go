// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflowplane

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeClusterWorkflowPlane = "clusterWorkflowPlane"
)

// clusterWorkflowPlaneServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type clusterWorkflowPlaneServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*clusterWorkflowPlaneServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a cluster workflow plane service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &clusterWorkflowPlaneServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *clusterWorkflowPlaneServiceWithAuthz) ListClusterWorkflowPlanes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterWorkflowPlane], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterWorkflowPlane], error) {
			return s.internal.ListClusterWorkflowPlanes(ctx, pageOpts)
		},
		func(cwp openchoreov1alpha1.ClusterWorkflowPlane) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewClusterWorkflowPlane,
				ResourceType: resourceTypeClusterWorkflowPlane,
				ResourceID:   cwp.Name,
				Hierarchy:    authz.ResourceHierarchy{},
			}
		},
	)
}

func (s *clusterWorkflowPlaneServiceWithAuthz) GetClusterWorkflowPlane(ctx context.Context, clusterWorkflowPlaneName string) (*openchoreov1alpha1.ClusterWorkflowPlane, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewClusterWorkflowPlane,
		ResourceType: resourceTypeClusterWorkflowPlane,
		ResourceID:   clusterWorkflowPlaneName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterWorkflowPlane(ctx, clusterWorkflowPlaneName)
}

// CreateClusterWorkflowPlane checks create authorization before delegating to the internal service.
func (s *clusterWorkflowPlaneServiceWithAuthz) CreateClusterWorkflowPlane(ctx context.Context, cwp *openchoreov1alpha1.ClusterWorkflowPlane) (*openchoreov1alpha1.ClusterWorkflowPlane, error) {
	if cwp == nil {
		return nil, ErrClusterWorkflowPlaneNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateClusterWorkflowPlane,
		ResourceType: resourceTypeClusterWorkflowPlane,
		ResourceID:   cwp.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateClusterWorkflowPlane(ctx, cwp)
}

// UpdateClusterWorkflowPlane checks update authorization before delegating to the internal service.
func (s *clusterWorkflowPlaneServiceWithAuthz) UpdateClusterWorkflowPlane(ctx context.Context, cwp *openchoreov1alpha1.ClusterWorkflowPlane) (*openchoreov1alpha1.ClusterWorkflowPlane, error) {
	if cwp == nil {
		return nil, ErrClusterWorkflowPlaneNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateClusterWorkflowPlane,
		ResourceType: resourceTypeClusterWorkflowPlane,
		ResourceID:   cwp.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateClusterWorkflowPlane(ctx, cwp)
}

// DeleteClusterWorkflowPlane checks delete authorization before delegating to the internal service.
func (s *clusterWorkflowPlaneServiceWithAuthz) DeleteClusterWorkflowPlane(ctx context.Context, clusterWorkflowPlaneName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteClusterWorkflowPlane,
		ResourceType: resourceTypeClusterWorkflowPlane,
		ResourceID:   clusterWorkflowPlaneName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return err
	}
	return s.internal.DeleteClusterWorkflowPlane(ctx, clusterWorkflowPlaneName)
}
