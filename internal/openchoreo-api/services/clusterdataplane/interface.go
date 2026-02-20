// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the cluster data plane service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	ListClusterDataPlanes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterDataPlane], error)
	GetClusterDataPlane(ctx context.Context, name string) (*openchoreov1alpha1.ClusterDataPlane, error)
	CreateClusterDataPlane(ctx context.Context, cdp *openchoreov1alpha1.ClusterDataPlane) (*openchoreov1alpha1.ClusterDataPlane, error)
}
