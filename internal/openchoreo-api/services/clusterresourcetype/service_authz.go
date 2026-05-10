// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterresourcetype

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeClusterResourceType = "clusterresourcetype"
)

// clusterResourceTypeServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type clusterResourceTypeServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*clusterResourceTypeServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a cluster resource type service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &clusterResourceTypeServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *clusterResourceTypeServiceWithAuthz) CreateClusterResourceType(ctx context.Context, crt *openchoreov1alpha1.ClusterResourceType) (*openchoreov1alpha1.ClusterResourceType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateClusterResourceType,
		ResourceType: resourceTypeClusterResourceType,
		ResourceID:   crt.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateClusterResourceType(ctx, crt)
}

func (s *clusterResourceTypeServiceWithAuthz) UpdateClusterResourceType(ctx context.Context, crt *openchoreov1alpha1.ClusterResourceType) (*openchoreov1alpha1.ClusterResourceType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateClusterResourceType,
		ResourceType: resourceTypeClusterResourceType,
		ResourceID:   crt.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateClusterResourceType(ctx, crt)
}

func (s *clusterResourceTypeServiceWithAuthz) ListClusterResourceTypes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterResourceType], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterResourceType], error) {
			return s.internal.ListClusterResourceTypes(ctx, pageOpts)
		},
		func(crt openchoreov1alpha1.ClusterResourceType) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewClusterResourceType,
				ResourceType: resourceTypeClusterResourceType,
				ResourceID:   crt.Name,
				Hierarchy:    authz.ResourceHierarchy{},
			}
		},
	)
}

func (s *clusterResourceTypeServiceWithAuthz) DeleteClusterResourceType(ctx context.Context, crtName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteClusterResourceType,
		ResourceType: resourceTypeClusterResourceType,
		ResourceID:   crtName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return err
	}
	return s.internal.DeleteClusterResourceType(ctx, crtName)
}

func (s *clusterResourceTypeServiceWithAuthz) GetClusterResourceType(ctx context.Context, crtName string) (*openchoreov1alpha1.ClusterResourceType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewClusterResourceType,
		ResourceType: resourceTypeClusterResourceType,
		ResourceID:   crtName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterResourceType(ctx, crtName)
}

func (s *clusterResourceTypeServiceWithAuthz) GetClusterResourceTypeSchema(ctx context.Context, crtName string) (map[string]any, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewClusterResourceType,
		ResourceType: resourceTypeClusterResourceType,
		ResourceID:   crtName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterResourceTypeSchema(ctx, crtName)
}
