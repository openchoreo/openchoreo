// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcetype

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeResourceType = "resourcetype"
)

// resourceTypeServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type resourceTypeServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*resourceTypeServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a resource type service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &resourceTypeServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *resourceTypeServiceWithAuthz) CreateResourceType(ctx context.Context, namespaceName string, rt *openchoreov1alpha1.ResourceType) (*openchoreov1alpha1.ResourceType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateResourceType,
		ResourceType: resourceTypeResourceType,
		ResourceID:   rt.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateResourceType(ctx, namespaceName, rt)
}

func (s *resourceTypeServiceWithAuthz) UpdateResourceType(ctx context.Context, namespaceName string, rt *openchoreov1alpha1.ResourceType) (*openchoreov1alpha1.ResourceType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateResourceType,
		ResourceType: resourceTypeResourceType,
		ResourceID:   rt.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateResourceType(ctx, namespaceName, rt)
}

func (s *resourceTypeServiceWithAuthz) ListResourceTypes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceType], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceType], error) {
			return s.internal.ListResourceTypes(ctx, namespaceName, pageOpts)
		},
		func(rt openchoreov1alpha1.ResourceType) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewResourceType,
				ResourceType: resourceTypeResourceType,
				ResourceID:   rt.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *resourceTypeServiceWithAuthz) GetResourceType(ctx context.Context, namespaceName, rtName string) (*openchoreov1alpha1.ResourceType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewResourceType,
		ResourceType: resourceTypeResourceType,
		ResourceID:   rtName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetResourceType(ctx, namespaceName, rtName)
}

func (s *resourceTypeServiceWithAuthz) DeleteResourceType(ctx context.Context, namespaceName, rtName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteResourceType,
		ResourceType: resourceTypeResourceType,
		ResourceID:   rtName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteResourceType(ctx, namespaceName, rtName)
}

func (s *resourceTypeServiceWithAuthz) GetResourceTypeSchema(ctx context.Context, namespaceName, rtName string) (map[string]any, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewResourceType,
		ResourceType: resourceTypeResourceType,
		ResourceID:   rtName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetResourceTypeSchema(ctx, namespaceName, rtName)
}
