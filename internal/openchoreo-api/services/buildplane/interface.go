// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the build plane service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	ListBuildPlanes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.BuildPlane], error)
	GetBuildPlane(ctx context.Context, namespaceName, buildPlaneName string) (*openchoreov1alpha1.BuildPlane, error)

	// GetBuildPlaneClient creates a Kubernetes client for the build plane cluster
	// via the cluster gateway proxy. Used by internal services for cross-cluster operations.
	GetBuildPlaneClient(ctx context.Context, namespaceName, gatewayURL string) (client.Client, error)

	// ArgoWorkflowExists checks whether the Argo Workflow referenced by runReference
	// still exists on the build plane.  Used by internal services
	ArgoWorkflowExists(ctx context.Context, namespaceName, gatewayURL string, runReference *openchoreov1alpha1.ResourceReference) bool
}
