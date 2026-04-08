// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowplane

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the workflow plane service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	ListWorkflowPlanes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.WorkflowPlane], error)
	GetWorkflowPlane(ctx context.Context, namespaceName, workflowPlaneName string) (*openchoreov1alpha1.WorkflowPlane, error)
	CreateWorkflowPlane(ctx context.Context, namespaceName string, wp *openchoreov1alpha1.WorkflowPlane) (*openchoreov1alpha1.WorkflowPlane, error)
	UpdateWorkflowPlane(ctx context.Context, namespaceName string, wp *openchoreov1alpha1.WorkflowPlane) (*openchoreov1alpha1.WorkflowPlane, error)
	DeleteWorkflowPlane(ctx context.Context, namespaceName, workflowPlaneName string) error

	// GetWorkflowPlaneClient creates a Kubernetes client for the workflow plane cluster.
	// Used by internal services for cross-cluster operations.
	GetWorkflowPlaneClient(ctx context.Context, namespaceName string) (client.Client, error)

	// ArgoWorkflowExists checks whether the Argo Workflow referenced by runReference
	// still exists on the workflow plane. Used by internal services.
	ArgoWorkflowExists(ctx context.Context, namespaceName string, runReference *openchoreov1alpha1.ResourceReference) bool
}
