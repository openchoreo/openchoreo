// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projecttype

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeProjectType = "projecttype"
)

// projectTypeServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type projectTypeServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*projectTypeServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a project type service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &projectTypeServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *projectTypeServiceWithAuthz) CreateProjectType(ctx context.Context, namespaceName string, pt *openchoreov1alpha1.ProjectType) (*openchoreov1alpha1.ProjectType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateProjectType,
		ResourceType: resourceTypeProjectType,
		ResourceID:   pt.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateProjectType(ctx, namespaceName, pt)
}

func (s *projectTypeServiceWithAuthz) UpdateProjectType(ctx context.Context, namespaceName string, pt *openchoreov1alpha1.ProjectType) (*openchoreov1alpha1.ProjectType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateProjectType,
		ResourceType: resourceTypeProjectType,
		ResourceID:   pt.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateProjectType(ctx, namespaceName, pt)
}

func (s *projectTypeServiceWithAuthz) ListProjectTypes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ProjectType], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ProjectType], error) {
			return s.internal.ListProjectTypes(ctx, namespaceName, pageOpts)
		},
		func(pt openchoreov1alpha1.ProjectType) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewProjectType,
				ResourceType: resourceTypeProjectType,
				ResourceID:   pt.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *projectTypeServiceWithAuthz) GetProjectType(ctx context.Context, namespaceName, ptName string) (*openchoreov1alpha1.ProjectType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewProjectType,
		ResourceType: resourceTypeProjectType,
		ResourceID:   ptName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetProjectType(ctx, namespaceName, ptName)
}

func (s *projectTypeServiceWithAuthz) DeleteProjectType(ctx context.Context, namespaceName, ptName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteProjectType,
		ResourceType: resourceTypeProjectType,
		ResourceID:   ptName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteProjectType(ctx, namespaceName, ptName)
}

func (s *projectTypeServiceWithAuthz) GetProjectTypeSchema(ctx context.Context, namespaceName, ptName string) (map[string]any, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewProjectType,
		ResourceType: resourceTypeProjectType,
		ResourceID:   ptName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetProjectTypeSchema(ctx, namespaceName, ptName)
}
