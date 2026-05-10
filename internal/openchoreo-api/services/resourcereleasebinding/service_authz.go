// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeResourceReleaseBinding = "resourcereleasebinding"
)

// resourceReleaseBindingServiceWithAuthz wraps a Service and adds authorization checks.
type resourceReleaseBindingServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*resourceReleaseBindingServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a resource release binding service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &resourceReleaseBindingServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *resourceReleaseBindingServiceWithAuthz) CreateResourceReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ResourceReleaseBinding) (*openchoreov1alpha1.ResourceReleaseBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateResourceReleaseBinding,
		ResourceType: resourceTypeResourceReleaseBinding,
		ResourceID:   rb.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rb.Spec.Owner.ProjectName,
		},
		Context: authz.Context{
			Resource: authz.ResourceAttribute{Environment: services.FormatDualScopedResourceName(namespaceName, rb.Spec.Environment, false)},
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateResourceReleaseBinding(ctx, namespaceName, rb)
}

func (s *resourceReleaseBindingServiceWithAuthz) UpdateResourceReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ResourceReleaseBinding) (*openchoreov1alpha1.ResourceReleaseBinding, error) {
	// Fetch the existing binding so authz uses the on-disk owner/environment
	// rather than whatever the client sent in the body. This prevents a caller
	// from claiming a different project in the body to bypass the project-scoped
	// authz check. Mirrors releasebinding's update path.
	existing, err := s.internal.GetResourceReleaseBinding(ctx, namespaceName, rb.Name)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateResourceReleaseBinding,
		ResourceType: resourceTypeResourceReleaseBinding,
		ResourceID:   rb.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   existing.Spec.Owner.ProjectName,
		},
		Context: authz.Context{
			Resource: authz.ResourceAttribute{Environment: services.FormatDualScopedResourceName(namespaceName, existing.Spec.Environment, false)},
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateResourceReleaseBinding(ctx, namespaceName, rb)
}

func (s *resourceReleaseBindingServiceWithAuthz) ListResourceReleaseBindings(ctx context.Context, namespaceName, resourceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceReleaseBinding], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceReleaseBinding], error) {
			return s.internal.ListResourceReleaseBindings(ctx, namespaceName, resourceName, pageOpts)
		},
		func(rb openchoreov1alpha1.ResourceReleaseBinding) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewResourceReleaseBinding,
				ResourceType: resourceTypeResourceReleaseBinding,
				ResourceID:   rb.Name,
				Hierarchy: authz.ResourceHierarchy{
					Namespace: namespaceName,
					Project:   rb.Spec.Owner.ProjectName,
				},
				Context: authz.Context{
					Resource: authz.ResourceAttribute{Environment: services.FormatDualScopedResourceName(namespaceName, rb.Spec.Environment, false)},
				},
			}
		},
	)
}

func (s *resourceReleaseBindingServiceWithAuthz) GetResourceReleaseBinding(ctx context.Context, namespaceName, resourceReleaseBindingName string) (*openchoreov1alpha1.ResourceReleaseBinding, error) {
	// Fetch first to get owner info for authz hierarchy
	rb, err := s.internal.GetResourceReleaseBinding(ctx, namespaceName, resourceReleaseBindingName)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewResourceReleaseBinding,
		ResourceType: resourceTypeResourceReleaseBinding,
		ResourceID:   resourceReleaseBindingName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rb.Spec.Owner.ProjectName,
		},
		Context: authz.Context{
			Resource: authz.ResourceAttribute{Environment: services.FormatDualScopedResourceName(namespaceName, rb.Spec.Environment, false)},
		},
	}); err != nil {
		return nil, err
	}
	return rb, nil
}

func (s *resourceReleaseBindingServiceWithAuthz) DeleteResourceReleaseBinding(ctx context.Context, namespaceName, resourceReleaseBindingName string) error {
	// Fetch first to get owner info for authz hierarchy
	rb, err := s.internal.GetResourceReleaseBinding(ctx, namespaceName, resourceReleaseBindingName)
	if err != nil {
		return err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteResourceReleaseBinding,
		ResourceType: resourceTypeResourceReleaseBinding,
		ResourceID:   resourceReleaseBindingName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rb.Spec.Owner.ProjectName,
		},
		Context: authz.Context{
			Resource: authz.ResourceAttribute{Environment: services.FormatDualScopedResourceName(namespaceName, rb.Spec.Environment, false)},
		},
	}); err != nil {
		return err
	}
	return s.internal.DeleteResourceReleaseBinding(ctx, namespaceName, resourceReleaseBindingName)
}
