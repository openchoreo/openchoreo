// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projecttype

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the project type service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	CreateProjectType(ctx context.Context, namespaceName string, pt *openchoreov1alpha1.ProjectType) (*openchoreov1alpha1.ProjectType, error)
	UpdateProjectType(ctx context.Context, namespaceName string, pt *openchoreov1alpha1.ProjectType) (*openchoreov1alpha1.ProjectType, error)
	ListProjectTypes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ProjectType], error)
	GetProjectType(ctx context.Context, namespaceName, ptName string) (*openchoreov1alpha1.ProjectType, error)
	DeleteProjectType(ctx context.Context, namespaceName, ptName string) error
	GetProjectTypeSchema(ctx context.Context, namespaceName, ptName string) (map[string]any, error)
}
