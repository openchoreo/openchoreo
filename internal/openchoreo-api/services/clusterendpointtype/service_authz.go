// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterendpointtype

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeClusterEndpointType = "clusterendpointtype"
)

// clusterEndpointTypeServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type clusterEndpointTypeServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*clusterEndpointTypeServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a cluster endpoint type service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &clusterEndpointTypeServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *clusterEndpointTypeServiceWithAuthz) CreateClusterEndpointType(ctx context.Context, crt *openchoreov1alpha1.ClusterEndpointType) (*openchoreov1alpha1.ClusterEndpointType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateClusterEndpointType,
		ResourceType: resourceTypeClusterEndpointType,
		ResourceID:   crt.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateClusterEndpointType(ctx, crt)
}

func (s *clusterEndpointTypeServiceWithAuthz) UpdateClusterEndpointType(ctx context.Context, crt *openchoreov1alpha1.ClusterEndpointType) (*openchoreov1alpha1.ClusterEndpointType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateClusterEndpointType,
		ResourceType: resourceTypeClusterEndpointType,
		ResourceID:   crt.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateClusterEndpointType(ctx, crt)
}

func (s *clusterEndpointTypeServiceWithAuthz) ListClusterEndpointTypes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterEndpointType], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterEndpointType], error) {
			return s.internal.ListClusterEndpointTypes(ctx, pageOpts)
		},
		func(crt openchoreov1alpha1.ClusterEndpointType) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewClusterEndpointType,
				ResourceType: resourceTypeClusterEndpointType,
				ResourceID:   crt.Name,
				Hierarchy:    authz.ResourceHierarchy{},
			}
		},
	)
}

func (s *clusterEndpointTypeServiceWithAuthz) DeleteClusterEndpointType(ctx context.Context, cetName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteClusterEndpointType,
		ResourceType: resourceTypeClusterEndpointType,
		ResourceID:   cetName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return err
	}
	return s.internal.DeleteClusterEndpointType(ctx, cetName)
}

func (s *clusterEndpointTypeServiceWithAuthz) GetClusterEndpointType(ctx context.Context, cetName string) (*openchoreov1alpha1.ClusterEndpointType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewClusterEndpointType,
		ResourceType: resourceTypeClusterEndpointType,
		ResourceID:   cetName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterEndpointType(ctx, cetName)
}

func (s *clusterEndpointTypeServiceWithAuthz) GetClusterEndpointTypeSchema(ctx context.Context, cetName string) (map[string]any, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewClusterEndpointType,
		ResourceType: resourceTypeClusterEndpointType,
		ResourceID:   cetName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterEndpointTypeSchema(ctx, cetName)
}
