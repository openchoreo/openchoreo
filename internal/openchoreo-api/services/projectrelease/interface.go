// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectrelease

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the project release service interface.
type Service interface {
	ListProjectReleases(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ProjectRelease], error)
	GetProjectRelease(ctx context.Context, namespaceName, projectReleaseName string) (*openchoreov1alpha1.ProjectRelease, error)
	CreateProjectRelease(ctx context.Context, namespaceName string, pr *openchoreov1alpha1.ProjectRelease) (*openchoreov1alpha1.ProjectRelease, error)
	DeleteProjectRelease(ctx context.Context, namespaceName, projectReleaseName string) error
}
