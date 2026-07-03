// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentreleasebinding

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeComponentReleaseBinding = "componentreleasebinding"
)

// componentReleaseBindingServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type componentReleaseBindingServiceWithAuthz struct {
	internal  Service
	k8sClient client.Client
	authz     *services.AuthzChecker
}

var _ Service = (*componentReleaseBindingServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a release binding service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &componentReleaseBindingServiceWithAuthz{
		internal:  NewService(k8sClient, logger),
		k8sClient: k8sClient,
		authz:     services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *componentReleaseBindingServiceWithAuthz) CreateComponentReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ComponentReleaseBinding) (*openchoreov1alpha1.ComponentReleaseBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateComponentReleaseBinding,
		ResourceType: resourceTypeComponentReleaseBinding,
		ResourceID:   rb.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rb.Spec.Owner.ProjectName,
			Component: rb.Spec.Owner.ComponentName,
		},
		Context: authz.Context{
			// TODO: pass kind discriminator once ComponentReleaseBindingSpec.Environment gains a kind field
			Resource: authz.ResourceAttribute{
				Environment: services.FormatDualScopedResourceName(namespaceName, rb.Spec.Environment, false)},
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateComponentReleaseBinding(ctx, namespaceName, rb)
}

func (s *componentReleaseBindingServiceWithAuthz) UpdateComponentReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ComponentReleaseBinding) (*openchoreov1alpha1.ComponentReleaseBinding, error) {
	// Fetch the existing release binding to get owner info for authz
	existing, err := s.internal.GetComponentReleaseBinding(ctx, namespaceName, rb.Name)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateComponentReleaseBinding,
		ResourceType: resourceTypeComponentReleaseBinding,
		ResourceID:   rb.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   existing.Spec.Owner.ProjectName,
			Component: existing.Spec.Owner.ComponentName,
		},
		Context: authz.Context{
			// TODO: pass kind discriminator once ComponentReleaseBindingSpec.Environment gains a kind field
			Resource: authz.ResourceAttribute{Environment: services.FormatDualScopedResourceName(namespaceName, existing.Spec.Environment, false)},
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateComponentReleaseBinding(ctx, namespaceName, rb)
}

func (s *componentReleaseBindingServiceWithAuthz) ListComponentComponentReleaseBindings(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ComponentReleaseBinding], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ComponentReleaseBinding], error) {
			return s.internal.ListComponentComponentReleaseBindings(ctx, namespaceName, componentName, pageOpts)
		},
		func(rb openchoreov1alpha1.ComponentReleaseBinding) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewComponentReleaseBinding,
				ResourceType: resourceTypeComponentReleaseBinding,
				ResourceID:   rb.Name,
				Hierarchy: authz.ResourceHierarchy{
					Namespace: namespaceName,
					Project:   rb.Spec.Owner.ProjectName,
					Component: rb.Spec.Owner.ComponentName,
				},
				Context: authz.Context{
					// TODO: pass kind discriminator once ComponentReleaseBindingSpec.Environment gains a kind field
					Resource: authz.ResourceAttribute{
						Environment: services.FormatDualScopedResourceName(namespaceName, rb.Spec.Environment, false)},
				},
			}
		},
	)
}

func (s *componentReleaseBindingServiceWithAuthz) GetComponentReleaseBinding(ctx context.Context, namespaceName, componentReleaseBindingName string) (*openchoreov1alpha1.ComponentReleaseBinding, error) {
	// Fetch the release binding first to get owner info for authz
	rb, err := s.internal.GetComponentReleaseBinding(ctx, namespaceName, componentReleaseBindingName)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewComponentReleaseBinding,
		ResourceType: resourceTypeComponentReleaseBinding,
		ResourceID:   componentReleaseBindingName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rb.Spec.Owner.ProjectName,
			Component: rb.Spec.Owner.ComponentName,
		},
		Context: authz.Context{
			// TODO: pass kind discriminator once ComponentReleaseBindingSpec.Environment gains a kind field
			Resource: authz.ResourceAttribute{
				Environment: services.FormatDualScopedResourceName(namespaceName, rb.Spec.Environment, false)},
		},
	}); err != nil {
		return nil, err
	}
	return rb, nil
}

func (s *componentReleaseBindingServiceWithAuthz) DeleteComponentReleaseBinding(ctx context.Context, namespaceName, componentReleaseBindingName string) error {
	// Fetch the release binding first to get owner info for authz
	rb, err := s.internal.GetComponentReleaseBinding(ctx, namespaceName, componentReleaseBindingName)
	if err != nil {
		return err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteComponentReleaseBinding,
		ResourceType: resourceTypeComponentReleaseBinding,
		ResourceID:   componentReleaseBindingName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rb.Spec.Owner.ProjectName,
			Component: rb.Spec.Owner.ComponentName,
		},
		Context: authz.Context{
			// TODO: pass kind discriminator once ComponentReleaseBindingSpec.Environment gains a kind field
			Resource: authz.ResourceAttribute{
				Environment: services.FormatDualScopedResourceName(namespaceName, rb.Spec.Environment, false)},
		},
	}); err != nil {
		return err
	}
	return s.internal.DeleteComponentReleaseBinding(ctx, namespaceName, componentReleaseBindingName)
}
