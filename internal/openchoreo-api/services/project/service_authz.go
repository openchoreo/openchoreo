// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"errors"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateProject = "project:create"
	actionUpdateProject = "project:update"
	actionViewProject   = "project:view"
	actionDeleteProject = "project:delete"

	resourceTypeProject = "project"
)

// projectServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type projectServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*projectServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a project service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &projectServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *projectServiceWithAuthz) CreateProject(ctx context.Context, namespaceName string, project *openchoreov1alpha1.Project) (*openchoreov1alpha1.Project, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateProject,
		ResourceType: resourceTypeProject,
		ResourceID:   project.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName, Project: project.Name},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateProject(ctx, namespaceName, project)
}

func (s *projectServiceWithAuthz) UpdateProject(ctx context.Context, namespaceName string, project *openchoreov1alpha1.Project) (*openchoreov1alpha1.Project, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateProject,
		ResourceType: resourceTypeProject,
		ResourceID:   project.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName, Project: project.Name},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateProject(ctx, namespaceName, project)
}

func (s *projectServiceWithAuthz) ListProjects(ctx context.Context, namespaceName string) ([]openchoreov1alpha1.Project, error) {
	allProjects, err := s.internal.ListProjects(ctx, namespaceName)
	if err != nil {
		return nil, err
	}

	authorized := make([]openchoreov1alpha1.Project, 0, len(allProjects))
	for _, p := range allProjects {
		if err := s.authz.Check(ctx, services.CheckRequest{
			Action:       actionViewProject,
			ResourceType: resourceTypeProject,
			ResourceID:   p.Name,
			Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName, Project: p.Name},
		}); err != nil {
			if errors.Is(err, services.ErrForbidden) {
				continue
			}
			return nil, err
		}
		authorized = append(authorized, p)
	}

	return authorized, nil
}

func (s *projectServiceWithAuthz) GetProject(ctx context.Context, namespaceName, projectName string) (*openchoreov1alpha1.Project, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewProject,
		ResourceType: resourceTypeProject,
		ResourceID:   projectName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetProject(ctx, namespaceName, projectName)
}

func (s *projectServiceWithAuthz) DeleteProject(ctx context.Context, namespaceName, projectName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteProject,
		ResourceType: resourceTypeProject,
		ResourceID:   projectName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteProject(ctx, namespaceName, projectName)
}
