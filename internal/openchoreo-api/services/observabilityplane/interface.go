// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the observability plane service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	ListObservabilityPlanes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ObservabilityPlane], error)
	GetObservabilityPlane(ctx context.Context, namespaceName, observabilityPlaneName string) (*openchoreov1alpha1.ObservabilityPlane, error)
	CreateObservabilityPlane(ctx context.Context, namespaceName string, op *openchoreov1alpha1.ObservabilityPlane) (*openchoreov1alpha1.ObservabilityPlane, error)
	UpdateObservabilityPlane(ctx context.Context, namespaceName string, op *openchoreov1alpha1.ObservabilityPlane) (*openchoreov1alpha1.ObservabilityPlane, error)
	DeleteObservabilityPlane(ctx context.Context, namespaceName, observabilityPlaneName string) error
}
