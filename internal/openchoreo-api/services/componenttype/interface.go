// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the component type service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design (discussion #1716).
type Service interface {
	CreateComponentType(ctx context.Context, namespaceName string, ct *openchoreov1alpha1.ComponentType) (*openchoreov1alpha1.ComponentType, error)
	UpdateComponentType(ctx context.Context, namespaceName string, ct *openchoreov1alpha1.ComponentType) (*openchoreov1alpha1.ComponentType, error)
	ListComponentTypes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ComponentType], error)
	GetComponentType(ctx context.Context, namespaceName, ctName string) (*openchoreov1alpha1.ComponentType, error)
	DeleteComponentType(ctx context.Context, namespaceName, ctName string) error
}
