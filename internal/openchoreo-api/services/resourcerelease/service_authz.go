// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcerelease

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeResourceRelease = "resourcerelease"
)

// resourceReleaseServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type resourceReleaseServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*resourceReleaseServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a resource release service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &resourceReleaseServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *resourceReleaseServiceWithAuthz) ListResourceReleases(ctx context.Context, namespaceName, resourceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceRelease], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceRelease], error) {
			return s.internal.ListResourceReleases(ctx, namespaceName, resourceName, pageOpts)
		},
		func(rr openchoreov1alpha1.ResourceRelease) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewResourceRelease,
				ResourceType: resourceTypeResourceRelease,
				ResourceID:   rr.Name,
				Hierarchy: authz.ResourceHierarchy{
					Namespace: namespaceName,
					Project:   rr.Spec.Owner.ProjectName,
					Resource:  rr.Spec.Owner.ResourceName,
				},
			}
		},
	)
}

func (s *resourceReleaseServiceWithAuthz) GetResourceRelease(ctx context.Context, namespaceName, resourceReleaseName string) (*openchoreov1alpha1.ResourceRelease, error) {
	// Fetch the resource release first to get owner info for authz
	rr, err := s.internal.GetResourceRelease(ctx, namespaceName, resourceReleaseName)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewResourceRelease,
		ResourceType: resourceTypeResourceRelease,
		ResourceID:   resourceReleaseName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rr.Spec.Owner.ProjectName,
			Resource:  rr.Spec.Owner.ResourceName,
		},
	}); err != nil {
		return nil, err
	}
	return rr, nil
}

func (s *resourceReleaseServiceWithAuthz) CreateResourceRelease(ctx context.Context, namespaceName string, rr *openchoreov1alpha1.ResourceRelease) (*openchoreov1alpha1.ResourceRelease, error) {
	if rr == nil {
		return nil, ErrResourceReleaseNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateResourceRelease,
		ResourceType: resourceTypeResourceRelease,
		ResourceID:   rr.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rr.Spec.Owner.ProjectName,
			Resource:  rr.Spec.Owner.ResourceName,
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateResourceRelease(ctx, namespaceName, rr)
}

func (s *resourceReleaseServiceWithAuthz) DeleteResourceRelease(ctx context.Context, namespaceName, resourceReleaseName string) error {
	// Fetch first to get owner info for authz
	rr, err := s.internal.GetResourceRelease(ctx, namespaceName, resourceReleaseName)
	if err != nil {
		return err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteResourceRelease,
		ResourceType: resourceTypeResourceRelease,
		ResourceID:   resourceReleaseName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rr.Spec.Owner.ProjectName,
			Resource:  rr.Spec.Owner.ResourceName,
		},
	}); err != nil {
		return err
	}
	return s.internal.DeleteResourceRelease(ctx, namespaceName, resourceReleaseName)
}
