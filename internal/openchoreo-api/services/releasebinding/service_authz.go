// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateReleaseBinding = "releasebinding:create"
	actionUpdateReleaseBinding = "releasebinding:update"
	actionViewReleaseBinding   = "releasebinding:view"
	actionDeleteReleaseBinding = "releasebinding:delete"

	resourceTypeReleaseBinding = "releasebinding"
)

// releaseBindingServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type releaseBindingServiceWithAuthz struct {
	internal  Service
	k8sClient client.Client
	authz     *services.AuthzChecker
}

var _ Service = (*releaseBindingServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a release binding service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &releaseBindingServiceWithAuthz{
		internal:  NewService(k8sClient, logger),
		k8sClient: k8sClient,
		authz:     services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *releaseBindingServiceWithAuthz) CreateReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ReleaseBinding) (*openchoreov1alpha1.ReleaseBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateReleaseBinding,
		ResourceType: resourceTypeReleaseBinding,
		ResourceID:   rb.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rb.Spec.Owner.ProjectName,
			Component: rb.Spec.Owner.ComponentName,
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateReleaseBinding(ctx, namespaceName, rb)
}

func (s *releaseBindingServiceWithAuthz) UpdateReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ReleaseBinding) (*openchoreov1alpha1.ReleaseBinding, error) {
	// Fetch the existing release binding to get owner info for authz
	existing, err := s.internal.GetReleaseBinding(ctx, namespaceName, rb.Name)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateReleaseBinding,
		ResourceType: resourceTypeReleaseBinding,
		ResourceID:   rb.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   existing.Spec.Owner.ProjectName,
			Component: existing.Spec.Owner.ComponentName,
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateReleaseBinding(ctx, namespaceName, rb)
}

func (s *releaseBindingServiceWithAuthz) ListReleaseBindings(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseBinding], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseBinding], error) {
			return s.internal.ListReleaseBindings(ctx, namespaceName, componentName, pageOpts)
		},
		func(rb openchoreov1alpha1.ReleaseBinding) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewReleaseBinding,
				ResourceType: resourceTypeReleaseBinding,
				ResourceID:   rb.Name,
				Hierarchy: authz.ResourceHierarchy{
					Namespace: namespaceName,
					Project:   rb.Spec.Owner.ProjectName,
					Component: rb.Spec.Owner.ComponentName,
				},
			}
		},
	)
}

func (s *releaseBindingServiceWithAuthz) GetReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string) (*openchoreov1alpha1.ReleaseBinding, error) {
	// Fetch the release binding first to get owner info for authz
	rb, err := s.internal.GetReleaseBinding(ctx, namespaceName, releaseBindingName)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewReleaseBinding,
		ResourceType: resourceTypeReleaseBinding,
		ResourceID:   releaseBindingName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rb.Spec.Owner.ProjectName,
			Component: rb.Spec.Owner.ComponentName,
		},
	}); err != nil {
		return nil, err
	}
	return rb, nil
}

func (s *releaseBindingServiceWithAuthz) DeleteReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string) error {
	// Fetch the release binding first to get owner info for authz
	rb, err := s.internal.GetReleaseBinding(ctx, namespaceName, releaseBindingName)
	if err != nil {
		return err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteReleaseBinding,
		ResourceType: resourceTypeReleaseBinding,
		ResourceID:   releaseBindingName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rb.Spec.Owner.ProjectName,
			Component: rb.Spec.Owner.ComponentName,
		},
	}); err != nil {
		return err
	}
	return s.internal.DeleteReleaseBinding(ctx, namespaceName, releaseBindingName)
}
