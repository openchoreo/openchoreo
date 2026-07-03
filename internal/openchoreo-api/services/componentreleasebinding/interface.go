// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentreleasebinding

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the release binding service interface.
type Service interface {
	CreateComponentReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ComponentReleaseBinding) (*openchoreov1alpha1.ComponentReleaseBinding, error)
	UpdateComponentReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ComponentReleaseBinding) (*openchoreov1alpha1.ComponentReleaseBinding, error)
	ListComponentComponentReleaseBindings(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ComponentReleaseBinding], error)
	GetComponentReleaseBinding(ctx context.Context, namespaceName, componentReleaseBindingName string) (*openchoreov1alpha1.ComponentReleaseBinding, error)
	DeleteComponentReleaseBinding(ctx context.Context, namespaceName, componentReleaseBindingName string) error
}
