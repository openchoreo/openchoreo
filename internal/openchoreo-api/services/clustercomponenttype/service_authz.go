// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

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
	actionViewClusterComponentType = "clustercomponenttype:view"

	resourceTypeClusterComponentType = "clusterComponentType"
)

// clusterComponentTypeServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type clusterComponentTypeServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*clusterComponentTypeServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a cluster component type service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &clusterComponentTypeServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *clusterComponentTypeServiceWithAuthz) ListClusterComponentTypes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterComponentType], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterComponentType], error) {
			return s.internal.ListClusterComponentTypes(ctx, pageOpts)
		},
		func(cct openchoreov1alpha1.ClusterComponentType) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewClusterComponentType,
				ResourceType: resourceTypeClusterComponentType,
				ResourceID:   cct.Name,
				Hierarchy:    authz.ResourceHierarchy{},
			}
		},
	)
}

func (s *clusterComponentTypeServiceWithAuthz) GetClusterComponentType(ctx context.Context, cctName string) (*openchoreov1alpha1.ClusterComponentType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewClusterComponentType,
		ResourceType: resourceTypeClusterComponentType,
		ResourceID:   cctName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterComponentType(ctx, cctName)
}

func (s *clusterComponentTypeServiceWithAuthz) GetClusterComponentTypeSchema(ctx context.Context, cctName string) (*extv1.JSONSchemaProps, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewClusterComponentType,
		ResourceType: resourceTypeClusterComponentType,
		ResourceID:   cctName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterComponentTypeSchema(ctx, cctName)
}
