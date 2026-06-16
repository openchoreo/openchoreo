// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectrelease

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeProjectRelease = "projectrelease"
)

// projectReleaseServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type projectReleaseServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*projectReleaseServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a project release service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &projectReleaseServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *projectReleaseServiceWithAuthz) ListProjectReleases(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ProjectRelease], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ProjectRelease], error) {
			return s.internal.ListProjectReleases(ctx, namespaceName, projectName, pageOpts)
		},
		func(pr openchoreov1alpha1.ProjectRelease) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewProjectRelease,
				ResourceType: resourceTypeProjectRelease,
				ResourceID:   pr.Name,
				Hierarchy: authz.ResourceHierarchy{
					Namespace: namespaceName,
					Project:   pr.Spec.Owner.ProjectName,
				},
			}
		},
	)
}

func (s *projectReleaseServiceWithAuthz) GetProjectRelease(ctx context.Context, namespaceName, projectReleaseName string) (*openchoreov1alpha1.ProjectRelease, error) {
	// Fetch the project release first to get owner info for authz
	pr, err := s.internal.GetProjectRelease(ctx, namespaceName, projectReleaseName)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewProjectRelease,
		ResourceType: resourceTypeProjectRelease,
		ResourceID:   projectReleaseName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   pr.Spec.Owner.ProjectName,
		},
	}); err != nil {
		return nil, err
	}
	return pr, nil
}

func (s *projectReleaseServiceWithAuthz) CreateProjectRelease(ctx context.Context, namespaceName string, pr *openchoreov1alpha1.ProjectRelease) (*openchoreov1alpha1.ProjectRelease, error) {
	if pr == nil {
		return nil, ErrProjectReleaseNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateProjectRelease,
		ResourceType: resourceTypeProjectRelease,
		ResourceID:   pr.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   pr.Spec.Owner.ProjectName,
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateProjectRelease(ctx, namespaceName, pr)
}

func (s *projectReleaseServiceWithAuthz) DeleteProjectRelease(ctx context.Context, namespaceName, projectReleaseName string) error {
	// Fetch first to get owner info for authz
	pr, err := s.internal.GetProjectRelease(ctx, namespaceName, projectReleaseName)
	if err != nil {
		return err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteProjectRelease,
		ResourceType: resourceTypeProjectRelease,
		ResourceID:   projectReleaseName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   pr.Spec.Owner.ProjectName,
		},
	}); err != nil {
		return err
	}
	return s.internal.DeleteProjectRelease(ctx, namespaceName, projectReleaseName)
}
