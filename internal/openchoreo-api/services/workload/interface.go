// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the workload service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design (discussion #1716).
type Service interface {
	CreateWorkload(ctx context.Context, namespaceName string, w *openchoreov1alpha1.Workload) (*openchoreov1alpha1.Workload, error)
	UpdateWorkload(ctx context.Context, namespaceName string, w *openchoreov1alpha1.Workload) (*openchoreov1alpha1.Workload, error)
	ListWorkloads(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Workload], error)
	GetWorkload(ctx context.Context, namespaceName, workloadName string) (*openchoreov1alpha1.Workload, error)
	DeleteWorkload(ctx context.Context, namespaceName, workloadName string) error
}
