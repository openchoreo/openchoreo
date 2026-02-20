// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

import (
	"context"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the cluster component type service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	ListClusterComponentTypes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterComponentType], error)
	GetClusterComponentType(ctx context.Context, cctName string) (*openchoreov1alpha1.ClusterComponentType, error)
	GetClusterComponentTypeSchema(ctx context.Context, cctName string) (*extv1.JSONSchemaProps, error)
}
