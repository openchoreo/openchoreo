// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Service defines the project service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design (discussion #1716).
type Service interface {
	CreateProject(ctx context.Context, namespaceName string, project *openchoreov1alpha1.Project) (*openchoreov1alpha1.Project, error)
	UpdateProject(ctx context.Context, namespaceName string, project *openchoreov1alpha1.Project) (*openchoreov1alpha1.Project, error)
	ListProjects(ctx context.Context, namespaceName string) ([]openchoreov1alpha1.Project, error)
	GetProject(ctx context.Context, namespaceName, projectName string) (*openchoreov1alpha1.Project, error)
	DeleteProject(ctx context.Context, namespaceName, projectName string) error
}
