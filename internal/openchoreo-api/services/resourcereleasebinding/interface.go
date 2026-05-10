// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the resource release binding service interface.
type Service interface {
	CreateResourceReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ResourceReleaseBinding) (*openchoreov1alpha1.ResourceReleaseBinding, error)
	UpdateResourceReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ResourceReleaseBinding) (*openchoreov1alpha1.ResourceReleaseBinding, error)
	ListResourceReleaseBindings(ctx context.Context, namespaceName, resourceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceReleaseBinding], error)
	GetResourceReleaseBinding(ctx context.Context, namespaceName, resourceReleaseBindingName string) (*openchoreov1alpha1.ResourceReleaseBinding, error)
	DeleteResourceReleaseBinding(ctx context.Context, namespaceName, resourceReleaseBindingName string) error
}
