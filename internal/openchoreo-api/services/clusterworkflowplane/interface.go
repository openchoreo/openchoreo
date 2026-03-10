// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflowplane

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the cluster workflow plane service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	ListClusterWorkflowPlanes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterWorkflowPlane], error)
	GetClusterWorkflowPlane(ctx context.Context, clusterWorkflowPlaneName string) (*openchoreov1alpha1.ClusterWorkflowPlane, error)
	CreateClusterWorkflowPlane(ctx context.Context, cwp *openchoreov1alpha1.ClusterWorkflowPlane) (*openchoreov1alpha1.ClusterWorkflowPlane, error)
	UpdateClusterWorkflowPlane(ctx context.Context, cwp *openchoreov1alpha1.ClusterWorkflowPlane) (*openchoreov1alpha1.ClusterWorkflowPlane, error)
	DeleteClusterWorkflowPlane(ctx context.Context, clusterWorkflowPlaneName string) error
}
