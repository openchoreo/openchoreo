// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterbuildplane

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionViewClusterBuildPlane   = "clusterbuildplane:view"
	actionUpdateClusterBuildPlane = "clusterbuildplane:update"
	actionDeleteClusterBuildPlane = "clusterbuildplane:delete"

	resourceTypeClusterBuildPlane = "clusterBuildPlane"
)

// clusterBuildPlaneServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type clusterBuildPlaneServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*clusterBuildPlaneServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a cluster build plane service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &clusterBuildPlaneServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *clusterBuildPlaneServiceWithAuthz) ListClusterBuildPlanes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterBuildPlane], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterBuildPlane], error) {
			return s.internal.ListClusterBuildPlanes(ctx, pageOpts)
		},
		func(cbp openchoreov1alpha1.ClusterBuildPlane) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewClusterBuildPlane,
				ResourceType: resourceTypeClusterBuildPlane,
				ResourceID:   cbp.Name,
				Hierarchy:    authz.ResourceHierarchy{},
			}
		},
	)
}

func (s *clusterBuildPlaneServiceWithAuthz) GetClusterBuildPlane(ctx context.Context, clusterBuildPlaneName string) (*openchoreov1alpha1.ClusterBuildPlane, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewClusterBuildPlane,
		ResourceType: resourceTypeClusterBuildPlane,
		ResourceID:   clusterBuildPlaneName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterBuildPlane(ctx, clusterBuildPlaneName)
}

// UpdateClusterBuildPlane checks update authorization before delegating to the internal service.
func (s *clusterBuildPlaneServiceWithAuthz) UpdateClusterBuildPlane(ctx context.Context, cbp *openchoreov1alpha1.ClusterBuildPlane) (*openchoreov1alpha1.ClusterBuildPlane, error) {
	if cbp == nil {
		return nil, ErrClusterBuildPlaneNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateClusterBuildPlane,
		ResourceType: resourceTypeClusterBuildPlane,
		ResourceID:   cbp.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateClusterBuildPlane(ctx, cbp)
}

// DeleteClusterBuildPlane checks delete authorization before delegating to the internal service.
func (s *clusterBuildPlaneServiceWithAuthz) DeleteClusterBuildPlane(ctx context.Context, clusterBuildPlaneName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteClusterBuildPlane,
		ResourceType: resourceTypeClusterBuildPlane,
		ResourceID:   clusterBuildPlaneName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return err
	}
	return s.internal.DeleteClusterBuildPlane(ctx, clusterBuildPlaneName)
}
