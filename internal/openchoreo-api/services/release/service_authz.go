// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionViewRelease   = "release:view"
	resourceTypeRelease = "release"
)

// releaseServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type releaseServiceWithAuthz struct {
	internal  Service
	k8sClient client.Client
	authz     *services.AuthzChecker
}

var _ Service = (*releaseServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a release service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &releaseServiceWithAuthz{
		internal:  NewService(k8sClient, logger),
		k8sClient: k8sClient,
		authz:     services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *releaseServiceWithAuthz) ListReleases(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Release], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Release], error) {
			return s.internal.ListReleases(ctx, namespaceName, componentName, pageOpts)
		},
		func(r openchoreov1alpha1.Release) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewRelease,
				ResourceType: resourceTypeRelease,
				ResourceID:   r.Name,
				Hierarchy: authz.ResourceHierarchy{
					Namespace: namespaceName,
					Project:   r.Spec.Owner.ProjectName,
					Component: r.Spec.Owner.ComponentName,
				},
			}
		},
	)
}

func (s *releaseServiceWithAuthz) GetRelease(ctx context.Context, namespaceName, releaseName string) (*openchoreov1alpha1.Release, error) {
	// Fetch the release first to get owner info for authz
	r, err := s.internal.GetRelease(ctx, namespaceName, releaseName)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewRelease,
		ResourceType: resourceTypeRelease,
		ResourceID:   releaseName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   r.Spec.Owner.ProjectName,
			Component: r.Spec.Owner.ComponentName,
		},
	}); err != nil {
		return nil, err
	}
	return r, nil
}
