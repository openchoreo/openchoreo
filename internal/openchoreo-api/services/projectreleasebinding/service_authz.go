// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeProjectReleaseBinding = "projectreleasebinding"
)

// projectReleaseBindingServiceWithAuthz wraps a Service and adds authorization checks.
type projectReleaseBindingServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*projectReleaseBindingServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a project release binding service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &projectReleaseBindingServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *projectReleaseBindingServiceWithAuthz) CreateProjectReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ProjectReleaseBinding) (*openchoreov1alpha1.ProjectReleaseBinding, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateProjectReleaseBinding,
		ResourceType: resourceTypeProjectReleaseBinding,
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
	return s.internal.CreateProjectReleaseBinding(ctx, namespaceName, rb)
}

func (s *projectReleaseBindingServiceWithAuthz) UpdateProjectReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ProjectReleaseBinding) (*openchoreov1alpha1.ProjectReleaseBinding, error) {
	// Fetch the existing binding so authz uses the on-disk owner/environment
	// rather than whatever the client sent in the body. This prevents a caller
	// from claiming a different project in the body to bypass the project-scoped
	// authz check. Mirrors resourcereleasebinding's update path.
	existing, err := s.internal.GetProjectReleaseBinding(ctx, namespaceName, rb.Name)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateProjectReleaseBinding,
		ResourceType: resourceTypeProjectReleaseBinding,
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
	return s.internal.UpdateProjectReleaseBinding(ctx, namespaceName, rb)
}

func (s *projectReleaseBindingServiceWithAuthz) ListProjectReleaseBindings(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ProjectReleaseBinding], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ProjectReleaseBinding], error) {
			return s.internal.ListProjectReleaseBindings(ctx, namespaceName, projectName, pageOpts)
		},
		func(rb openchoreov1alpha1.ProjectReleaseBinding) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewProjectReleaseBinding,
				ResourceType: resourceTypeProjectReleaseBinding,
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

func (s *projectReleaseBindingServiceWithAuthz) GetProjectReleaseBinding(ctx context.Context, namespaceName, projectReleaseBindingName string) (*openchoreov1alpha1.ProjectReleaseBinding, error) {
	// Fetch first to get owner info for authz hierarchy
	rb, err := s.internal.GetProjectReleaseBinding(ctx, namespaceName, projectReleaseBindingName)
	if err != nil {
		return nil, err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewProjectReleaseBinding,
		ResourceType: resourceTypeProjectReleaseBinding,
		ResourceID:   projectReleaseBindingName,
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

func (s *projectReleaseBindingServiceWithAuthz) DeleteProjectReleaseBinding(ctx context.Context, namespaceName, projectReleaseBindingName string) error {
	// Fetch first to get owner info for authz hierarchy
	rb, err := s.internal.GetProjectReleaseBinding(ctx, namespaceName, projectReleaseBindingName)
	if err != nil {
		return err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteProjectReleaseBinding,
		ResourceType: resourceTypeProjectReleaseBinding,
		ResourceID:   projectReleaseBindingName,
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
	return s.internal.DeleteProjectReleaseBinding(ctx, namespaceName, projectReleaseBindingName)
}
