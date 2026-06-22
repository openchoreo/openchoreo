// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the project release binding service interface.
type Service interface {
	CreateProjectReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ProjectReleaseBinding) (*openchoreov1alpha1.ProjectReleaseBinding, error)
	UpdateProjectReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ProjectReleaseBinding) (*openchoreov1alpha1.ProjectReleaseBinding, error)
	ListProjectReleaseBindings(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ProjectReleaseBinding], error)
	GetProjectReleaseBinding(ctx context.Context, namespaceName, projectReleaseBindingName string) (*openchoreov1alpha1.ProjectReleaseBinding, error)
	DeleteProjectReleaseBinding(ctx context.Context, namespaceName, projectReleaseBindingName string) error
}
