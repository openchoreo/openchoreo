// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionViewComponentRelease   = "componentrelease:view"
	resourceTypeComponentRelease = "componentrelease"
)

// componentReleaseServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type componentReleaseServiceWithAuthz struct {
	internal  Service
	k8sClient client.Client
	authz     *services.AuthzChecker
}

var _ Service = (*componentReleaseServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a component release service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &componentReleaseServiceWithAuthz{
		internal:  NewService(k8sClient, logger),
		k8sClient: k8sClient,
		authz:     services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *componentReleaseServiceWithAuthz) ListComponentReleases(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ComponentRelease], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ComponentRelease], error) {
			return s.internal.ListComponentReleases(ctx, namespaceName, componentName, pageOpts)
		},
		func(cr openchoreov1alpha1.ComponentRelease) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewComponentRelease,
				ResourceType: resourceTypeComponentRelease,
				ResourceID:   cr.Name,
				Hierarchy: authz.ResourceHierarchy{
					Namespace: namespaceName,
					Project:   cr.Spec.Owner.ProjectName,
					Component: cr.Spec.Owner.ComponentName,
				},
			}
		},
	)
}

func (s *componentReleaseServiceWithAuthz) GetComponentRelease(ctx context.Context, namespaceName, componentReleaseName string) (*openchoreov1alpha1.ComponentRelease, error) {
	// Fetch the component release first to get owner info for authz
	cr, err := s.internal.GetComponentRelease(ctx, namespaceName, componentReleaseName)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewComponentRelease,
		ResourceType: resourceTypeComponentRelease,
		ResourceID:   componentReleaseName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   cr.Spec.Owner.ProjectName,
			Component: cr.Spec.Owner.ComponentName,
		},
	}); err != nil {
		return nil, err
	}
	return cr, nil
}
