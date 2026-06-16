// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterprojecttype

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeClusterProjectType = "clusterprojecttype"
)

// clusterProjectTypeServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type clusterProjectTypeServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*clusterProjectTypeServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a cluster project type service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &clusterProjectTypeServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *clusterProjectTypeServiceWithAuthz) CreateClusterProjectType(ctx context.Context, cpt *openchoreov1alpha1.ClusterProjectType) (*openchoreov1alpha1.ClusterProjectType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateClusterProjectType,
		ResourceType: resourceTypeClusterProjectType,
		ResourceID:   cpt.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateClusterProjectType(ctx, cpt)
}

func (s *clusterProjectTypeServiceWithAuthz) UpdateClusterProjectType(ctx context.Context, cpt *openchoreov1alpha1.ClusterProjectType) (*openchoreov1alpha1.ClusterProjectType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateClusterProjectType,
		ResourceType: resourceTypeClusterProjectType,
		ResourceID:   cpt.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateClusterProjectType(ctx, cpt)
}

func (s *clusterProjectTypeServiceWithAuthz) ListClusterProjectTypes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterProjectType], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterProjectType], error) {
			return s.internal.ListClusterProjectTypes(ctx, pageOpts)
		},
		func(cpt openchoreov1alpha1.ClusterProjectType) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewClusterProjectType,
				ResourceType: resourceTypeClusterProjectType,
				ResourceID:   cpt.Name,
				Hierarchy:    authz.ResourceHierarchy{},
			}
		},
	)
}

func (s *clusterProjectTypeServiceWithAuthz) DeleteClusterProjectType(ctx context.Context, cptName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteClusterProjectType,
		ResourceType: resourceTypeClusterProjectType,
		ResourceID:   cptName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return err
	}
	return s.internal.DeleteClusterProjectType(ctx, cptName)
}

func (s *clusterProjectTypeServiceWithAuthz) GetClusterProjectType(ctx context.Context, cptName string) (*openchoreov1alpha1.ClusterProjectType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewClusterProjectType,
		ResourceType: resourceTypeClusterProjectType,
		ResourceID:   cptName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterProjectType(ctx, cptName)
}

func (s *clusterProjectTypeServiceWithAuthz) GetClusterProjectTypeSchema(ctx context.Context, cptName string) (map[string]any, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewClusterProjectType,
		ResourceType: resourceTypeClusterProjectType,
		ResourceID:   cptName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterProjectTypeSchema(ctx, cptName)
}
