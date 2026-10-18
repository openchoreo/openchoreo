// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrelease

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeEndpointRelease = "endpointrelease"
)

// endpointReleaseServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type endpointReleaseServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*endpointReleaseServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a endpoint release service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &endpointReleaseServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *endpointReleaseServiceWithAuthz) ListEndpointReleases(ctx context.Context, namespaceName, endpointName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.EndpointRelease], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.EndpointRelease], error) {
			return s.internal.ListEndpointReleases(ctx, namespaceName, endpointName, pageOpts)
		},
		func(rr openchoreov1alpha1.EndpointRelease) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewEndpointRelease,
				ResourceType: resourceTypeEndpointRelease,
				ResourceID:   rr.Name,
				Hierarchy: authz.ResourceHierarchy{
					Namespace: namespaceName,
					Project:   rr.Spec.Owner.ProjectName,
				},
			}
		},
	)
}

func (s *endpointReleaseServiceWithAuthz) GetEndpointRelease(ctx context.Context, namespaceName, endpointReleaseName string) (*openchoreov1alpha1.EndpointRelease, error) {
	// Fetch the endpoint release first to get owner info for authz
	rr, err := s.internal.GetEndpointRelease(ctx, namespaceName, endpointReleaseName)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewEndpointRelease,
		ResourceType: resourceTypeEndpointRelease,
		ResourceID:   endpointReleaseName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rr.Spec.Owner.ProjectName,
		},
	}); err != nil {
		return nil, err
	}
	return rr, nil
}

func (s *endpointReleaseServiceWithAuthz) DeleteEndpointRelease(ctx context.Context, namespaceName, endpointReleaseName string) error {
	// Fetch first to get owner info for authz
	rr, err := s.internal.GetEndpointRelease(ctx, namespaceName, endpointReleaseName)
	if err != nil {
		return err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteEndpointRelease,
		ResourceType: resourceTypeEndpointRelease,
		ResourceID:   endpointReleaseName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rr.Spec.Owner.ProjectName,
		},
	}); err != nil {
		return err
	}
	return s.internal.DeleteEndpointRelease(ctx, namespaceName, endpointReleaseName)
}
