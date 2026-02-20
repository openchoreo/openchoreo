// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the cluster observability plane service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	ListClusterObservabilityPlanes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterObservabilityPlane], error)
	GetClusterObservabilityPlane(ctx context.Context, clusterObservabilityPlaneName string) (*openchoreov1alpha1.ClusterObservabilityPlane, error)
}
