// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the data plane service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	ListDataPlanes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.DataPlane], error)
	GetDataPlane(ctx context.Context, namespaceName, dpName string) (*openchoreov1alpha1.DataPlane, error)
	CreateDataPlane(ctx context.Context, namespaceName string, dp *openchoreov1alpha1.DataPlane) (*openchoreov1alpha1.DataPlane, error)
	UpdateDataPlane(ctx context.Context, namespaceName string, dp *openchoreov1alpha1.DataPlane) (*openchoreov1alpha1.DataPlane, error)
	DeleteDataPlane(ctx context.Context, namespaceName, dpName string) error
}
