// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionViewClusterObservabilityPlane   = "clusterobservabilityplane:view"
	actionCreateClusterObservabilityPlane = "clusterobservabilityplane:create"
	actionUpdateClusterObservabilityPlane = "clusterobservabilityplane:update"
	actionDeleteClusterObservabilityPlane = "clusterobservabilityplane:delete"

	resourceTypeClusterObservabilityPlane = "clusterobservabilityplane"
)

// clusterObservabilityPlaneServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type clusterObservabilityPlaneServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*clusterObservabilityPlaneServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a cluster observability plane service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &clusterObservabilityPlaneServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *clusterObservabilityPlaneServiceWithAuthz) ListClusterObservabilityPlanes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterObservabilityPlane], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterObservabilityPlane], error) {
			return s.internal.ListClusterObservabilityPlanes(ctx, pageOpts)
		},
		func(cop openchoreov1alpha1.ClusterObservabilityPlane) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewClusterObservabilityPlane,
				ResourceType: resourceTypeClusterObservabilityPlane,
				ResourceID:   cop.Name,
				Hierarchy:    authz.ResourceHierarchy{},
			}
		},
	)
}

func (s *clusterObservabilityPlaneServiceWithAuthz) GetClusterObservabilityPlane(ctx context.Context, clusterObservabilityPlaneName string) (*openchoreov1alpha1.ClusterObservabilityPlane, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewClusterObservabilityPlane,
		ResourceType: resourceTypeClusterObservabilityPlane,
		ResourceID:   clusterObservabilityPlaneName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterObservabilityPlane(ctx, clusterObservabilityPlaneName)
}

// CreateClusterObservabilityPlane checks create authorization before delegating to the internal service.
func (s *clusterObservabilityPlaneServiceWithAuthz) CreateClusterObservabilityPlane(ctx context.Context, cop *openchoreov1alpha1.ClusterObservabilityPlane) (*openchoreov1alpha1.ClusterObservabilityPlane, error) {
	if cop == nil {
		return nil, ErrClusterObservabilityPlaneNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateClusterObservabilityPlane,
		ResourceType: resourceTypeClusterObservabilityPlane,
		ResourceID:   cop.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateClusterObservabilityPlane(ctx, cop)
}

// UpdateClusterObservabilityPlane checks update authorization before delegating to the internal service.
func (s *clusterObservabilityPlaneServiceWithAuthz) UpdateClusterObservabilityPlane(ctx context.Context, cop *openchoreov1alpha1.ClusterObservabilityPlane) (*openchoreov1alpha1.ClusterObservabilityPlane, error) {
	if cop == nil {
		return nil, ErrClusterObservabilityPlaneNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateClusterObservabilityPlane,
		ResourceType: resourceTypeClusterObservabilityPlane,
		ResourceID:   cop.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateClusterObservabilityPlane(ctx, cop)
}

// DeleteClusterObservabilityPlane checks delete authorization before delegating to the internal service.
func (s *clusterObservabilityPlaneServiceWithAuthz) DeleteClusterObservabilityPlane(ctx context.Context, clusterObservabilityPlaneName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteClusterObservabilityPlane,
		ResourceType: resourceTypeClusterObservabilityPlane,
		ResourceID:   clusterObservabilityPlaneName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return err
	}
	return s.internal.DeleteClusterObservabilityPlane(ctx, clusterObservabilityPlaneName)
}
