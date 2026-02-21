// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

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
	actionCreateClusterTrait = "clustertrait:create"
	actionUpdateClusterTrait = "clustertrait:update"
	actionDeleteClusterTrait = "clustertrait:delete"
	actionViewClusterTrait   = "clustertrait:view"

	resourceTypeClusterTrait = "clusterTrait"
)

// clusterTraitServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type clusterTraitServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*clusterTraitServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a cluster trait service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &clusterTraitServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *clusterTraitServiceWithAuthz) CreateClusterTrait(ctx context.Context, ct *openchoreov1alpha1.ClusterTrait) (*openchoreov1alpha1.ClusterTrait, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateClusterTrait,
		ResourceType: resourceTypeClusterTrait,
		ResourceID:   ct.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateClusterTrait(ctx, ct)
}

func (s *clusterTraitServiceWithAuthz) UpdateClusterTrait(ctx context.Context, ct *openchoreov1alpha1.ClusterTrait) (*openchoreov1alpha1.ClusterTrait, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateClusterTrait,
		ResourceType: resourceTypeClusterTrait,
		ResourceID:   ct.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateClusterTrait(ctx, ct)
}

func (s *clusterTraitServiceWithAuthz) ListClusterTraits(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterTrait], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterTrait], error) {
			return s.internal.ListClusterTraits(ctx, pageOpts)
		},
		func(ct openchoreov1alpha1.ClusterTrait) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewClusterTrait,
				ResourceType: resourceTypeClusterTrait,
				ResourceID:   ct.Name,
				Hierarchy:    authz.ResourceHierarchy{},
			}
		},
	)
}

// DeleteClusterTrait checks delete authorization before delegating to the internal service.
func (s *clusterTraitServiceWithAuthz) DeleteClusterTrait(ctx context.Context, clusterTraitName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteClusterTrait,
		ResourceType: resourceTypeClusterTrait,
		ResourceID:   clusterTraitName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return err
	}
	return s.internal.DeleteClusterTrait(ctx, clusterTraitName)
}

func (s *clusterTraitServiceWithAuthz) GetClusterTrait(ctx context.Context, clusterTraitName string) (*openchoreov1alpha1.ClusterTrait, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewClusterTrait,
		ResourceType: resourceTypeClusterTrait,
		ResourceID:   clusterTraitName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterTrait(ctx, clusterTraitName)
}

func (s *clusterTraitServiceWithAuthz) GetClusterTraitSchema(ctx context.Context, clusterTraitName string) (*extv1.JSONSchemaProps, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewClusterTrait,
		ResourceType: resourceTypeClusterTrait,
		ResourceID:   clusterTraitName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterTraitSchema(ctx, clusterTraitName)
}
