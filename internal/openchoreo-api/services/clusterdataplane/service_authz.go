// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionViewClusterDataPlane   = "clusterdataplane:view"
	actionCreateClusterDataPlane = "clusterdataplane:create"

	resourceTypeClusterDataPlane = "clusterDataPlane"
)

// clusterDataPlaneServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type clusterDataPlaneServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*clusterDataPlaneServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a cluster data plane service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &clusterDataPlaneServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *clusterDataPlaneServiceWithAuthz) ListClusterDataPlanes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterDataPlane], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterDataPlane], error) {
			return s.internal.ListClusterDataPlanes(ctx, pageOpts)
		},
		func(cdp openchoreov1alpha1.ClusterDataPlane) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewClusterDataPlane,
				ResourceType: resourceTypeClusterDataPlane,
				ResourceID:   cdp.Name,
				Hierarchy:    authz.ResourceHierarchy{},
			}
		},
	)
}

func (s *clusterDataPlaneServiceWithAuthz) GetClusterDataPlane(ctx context.Context, name string) (*openchoreov1alpha1.ClusterDataPlane, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewClusterDataPlane,
		ResourceType: resourceTypeClusterDataPlane,
		ResourceID:   name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterDataPlane(ctx, name)
}

func (s *clusterDataPlaneServiceWithAuthz) CreateClusterDataPlane(ctx context.Context, cdp *openchoreov1alpha1.ClusterDataPlane) (*openchoreov1alpha1.ClusterDataPlane, error) {
	if cdp == nil {
		return nil, ErrClusterDataPlaneNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateClusterDataPlane,
		ResourceType: resourceTypeClusterDataPlane,
		ResourceID:   cdp.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateClusterDataPlane(ctx, cdp)
}
