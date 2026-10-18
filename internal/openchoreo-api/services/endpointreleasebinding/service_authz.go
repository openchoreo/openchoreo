// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeEndpointReleaseBinding = "endpointreleasebinding"
)

// endpointReleaseBindingServiceWithAuthz wraps a Service and adds authorization checks.
type endpointReleaseBindingServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*endpointReleaseBindingServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a endpoint release binding service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &endpointReleaseBindingServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *endpointReleaseBindingServiceWithAuthz) CreateEndpointReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.EndpointReleaseBinding) (*openchoreov1alpha1.EndpointReleaseBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateEndpointReleaseBinding,
		ResourceType: resourceTypeEndpointReleaseBinding,
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
	return s.internal.CreateEndpointReleaseBinding(ctx, namespaceName, rb)
}

func (s *endpointReleaseBindingServiceWithAuthz) UpdateEndpointReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.EndpointReleaseBinding) (*openchoreov1alpha1.EndpointReleaseBinding, error) {
	// Fetch the existing binding so authz uses the on-disk owner/environment
	// rather than whatever the client sent in the body. This prevents a caller
	// from claiming a different project in the body to bypass the project-scoped
	// authz check. Mirrors releasebinding's update path.
	existing, err := s.internal.GetEndpointReleaseBinding(ctx, namespaceName, rb.Name)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateEndpointReleaseBinding,
		ResourceType: resourceTypeEndpointReleaseBinding,
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
	return s.internal.UpdateEndpointReleaseBinding(ctx, namespaceName, rb)
}

func (s *endpointReleaseBindingServiceWithAuthz) ListEndpointReleaseBindings(ctx context.Context, namespaceName, endpointName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.EndpointReleaseBinding], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.EndpointReleaseBinding], error) {
			return s.internal.ListEndpointReleaseBindings(ctx, namespaceName, endpointName, pageOpts)
		},
		func(rb openchoreov1alpha1.EndpointReleaseBinding) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewEndpointReleaseBinding,
				ResourceType: resourceTypeEndpointReleaseBinding,
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

func (s *endpointReleaseBindingServiceWithAuthz) GetEndpointReleaseBinding(ctx context.Context, namespaceName, endpointReleaseBindingName string) (*openchoreov1alpha1.EndpointReleaseBinding, error) {
	// Fetch first to get owner info for authz hierarchy
	rb, err := s.internal.GetEndpointReleaseBinding(ctx, namespaceName, endpointReleaseBindingName)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewEndpointReleaseBinding,
		ResourceType: resourceTypeEndpointReleaseBinding,
		ResourceID:   endpointReleaseBindingName,
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

func (s *endpointReleaseBindingServiceWithAuthz) DeleteEndpointReleaseBinding(ctx context.Context, namespaceName, endpointReleaseBindingName string) error {
	// Fetch first to get owner info for authz hierarchy
	rb, err := s.internal.GetEndpointReleaseBinding(ctx, namespaceName, endpointReleaseBindingName)
	if err != nil {
		return err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteEndpointReleaseBinding,
		ResourceType: resourceTypeEndpointReleaseBinding,
		ResourceID:   endpointReleaseBindingName,
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
	return s.internal.DeleteEndpointReleaseBinding(ctx, namespaceName, endpointReleaseBindingName)
}
