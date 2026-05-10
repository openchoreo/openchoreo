// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the resource service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	CreateResource(ctx context.Context, namespaceName string, resource *openchoreov1alpha1.Resource) (*openchoreov1alpha1.Resource, error)
	UpdateResource(ctx context.Context, namespaceName string, resource *openchoreov1alpha1.Resource) (*openchoreov1alpha1.Resource, error)
	ListResources(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Resource], error)
	GetResource(ctx context.Context, namespaceName, resourceName string) (*openchoreov1alpha1.Resource, error)
	DeleteResource(ctx context.Context, namespaceName, resourceName string) error
}
