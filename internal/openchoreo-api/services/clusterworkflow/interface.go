// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the cluster workflow service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	CreateClusterWorkflow(ctx context.Context, cwf *openchoreov1alpha1.ClusterWorkflow) (*openchoreov1alpha1.ClusterWorkflow, error)
	UpdateClusterWorkflow(ctx context.Context, cwf *openchoreov1alpha1.ClusterWorkflow) (*openchoreov1alpha1.ClusterWorkflow, error)
	ListClusterWorkflows(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterWorkflow], error)
	GetClusterWorkflow(ctx context.Context, clusterWorkflowName string) (*openchoreov1alpha1.ClusterWorkflow, error)
	DeleteClusterWorkflow(ctx context.Context, clusterWorkflowName string) error
	GetClusterWorkflowSchema(ctx context.Context, clusterWorkflowName string) (map[string]any, error)
}
