// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateComponent = "component:create"
	actionUpdateComponent = "component:update"
	actionViewComponent   = "component:view"
	actionDeleteComponent = "component:delete"

	resourceTypeComponent = "component"
)

// componentServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type componentServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*componentServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a component service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &componentServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *componentServiceWithAuthz) CreateComponent(ctx context.Context, namespaceName string, component *openchoreov1alpha1.Component) (*openchoreov1alpha1.Component, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateComponent,
		ResourceType: resourceTypeComponent,
		ResourceID:   component.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   component.Spec.Owner.ProjectName,
			Component: component.Name,
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateComponent(ctx, namespaceName, component)
}

func (s *componentServiceWithAuthz) UpdateComponent(ctx context.Context, namespaceName string, component *openchoreov1alpha1.Component) (*openchoreov1alpha1.Component, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateComponent,
		ResourceType: resourceTypeComponent,
		ResourceID:   component.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   component.Spec.Owner.ProjectName,
			Component: component.Name,
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateComponent(ctx, namespaceName, component)
}

func (s *componentServiceWithAuthz) ListComponents(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Component], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Component], error) {
			return s.internal.ListComponents(ctx, namespaceName, projectName, pageOpts)
		},
		func(c openchoreov1alpha1.Component) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewComponent,
				ResourceType: resourceTypeComponent,
				ResourceID:   c.Name,
				Hierarchy: authz.ResourceHierarchy{
					Namespace: namespaceName,
					Project:   c.Spec.Owner.ProjectName,
					Component: c.Name,
				},
			}
		},
	)
}

func (s *componentServiceWithAuthz) GetComponent(ctx context.Context, namespaceName, componentName string) (*openchoreov1alpha1.Component, error) {
	// Fetch first to get the project for authz hierarchy
	comp, err := s.internal.GetComponent(ctx, namespaceName, componentName)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewComponent,
		ResourceType: resourceTypeComponent,
		ResourceID:   componentName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   comp.Spec.Owner.ProjectName,
			Component: componentName,
		},
	}); err != nil {
		return nil, err
	}
	return comp, nil
}

func (s *componentServiceWithAuthz) DeleteComponent(ctx context.Context, namespaceName, componentName string) error {
	// Fetch first to get the project for authz hierarchy
	comp, err := s.internal.GetComponent(ctx, namespaceName, componentName)
	if err != nil {
		return err
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteComponent,
		ResourceType: resourceTypeComponent,
		ResourceID:   componentName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   comp.Spec.Owner.ProjectName,
			Component: componentName,
		},
	}); err != nil {
		return err
	}
	return s.internal.DeleteComponent(ctx, namespaceName, componentName)
}
