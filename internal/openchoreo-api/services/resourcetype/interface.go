// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcetype

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the resource type service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	CreateResourceType(ctx context.Context, namespaceName string, rt *openchoreov1alpha1.ResourceType) (*openchoreov1alpha1.ResourceType, error)
	UpdateResourceType(ctx context.Context, namespaceName string, rt *openchoreov1alpha1.ResourceType) (*openchoreov1alpha1.ResourceType, error)
	ListResourceTypes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceType], error)
	GetResourceType(ctx context.Context, namespaceName, rtName string) (*openchoreov1alpha1.ResourceType, error)
	DeleteResourceType(ctx context.Context, namespaceName, rtName string) error
	GetResourceTypeSchema(ctx context.Context, namespaceName, rtName string) (map[string]any, error)
}
