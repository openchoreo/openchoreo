// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeResource = "resource"
)

// resourceServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type resourceServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*resourceServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a resource service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &resourceServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *resourceServiceWithAuthz) CreateResource(ctx context.Context, namespaceName string, resource *openchoreov1alpha1.Resource) (*openchoreov1alpha1.Resource, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateResource,
		ResourceType: resourceTypeResource,
		ResourceID:   resource.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   resource.Spec.Owner.ProjectName,
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateResource(ctx, namespaceName, resource)
}

func (s *resourceServiceWithAuthz) UpdateResource(ctx context.Context, namespaceName string, resource *openchoreov1alpha1.Resource) (*openchoreov1alpha1.Resource, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateResource,
		ResourceType: resourceTypeResource,
		ResourceID:   resource.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   resource.Spec.Owner.ProjectName,
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateResource(ctx, namespaceName, resource)
}

func (s *resourceServiceWithAuthz) ListResources(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Resource], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Resource], error) {
			return s.internal.ListResources(ctx, namespaceName, projectName, pageOpts)
		},
		func(r openchoreov1alpha1.Resource) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewResource,
				ResourceType: resourceTypeResource,
				ResourceID:   r.Name,
				Hierarchy: authz.ResourceHierarchy{
					Namespace: namespaceName,
					Project:   r.Spec.Owner.ProjectName,
				},
			}
		},
	)
}

func (s *resourceServiceWithAuthz) GetResource(ctx context.Context, namespaceName, resourceName string) (*openchoreov1alpha1.Resource, error) {
	// Fetch first to get the project for authz hierarchy
	r, err := s.internal.GetResource(ctx, namespaceName, resourceName)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewResource,
		ResourceType: resourceTypeResource,
		ResourceID:   resourceName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   r.Spec.Owner.ProjectName,
		},
	}); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *resourceServiceWithAuthz) DeleteResource(ctx context.Context, namespaceName, resourceName string) error {
	// Fetch first to get the project for authz hierarchy
	r, err := s.internal.GetResource(ctx, namespaceName, resourceName)
	if err != nil {
		return err
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteResource,
		ResourceType: resourceTypeResource,
		ResourceID:   resourceName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   r.Spec.Owner.ProjectName,
		},
	}); err != nil {
		return err
	}
	return s.internal.DeleteResource(ctx, namespaceName, resourceName)
}
