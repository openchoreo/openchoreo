// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterendpointtype

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the cluster endpoint type service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	CreateClusterEndpointType(ctx context.Context, crt *openchoreov1alpha1.ClusterEndpointType) (*openchoreov1alpha1.ClusterEndpointType, error)
	UpdateClusterEndpointType(ctx context.Context, crt *openchoreov1alpha1.ClusterEndpointType) (*openchoreov1alpha1.ClusterEndpointType, error)
	ListClusterEndpointTypes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterEndpointType], error)
	GetClusterEndpointType(ctx context.Context, cetName string) (*openchoreov1alpha1.ClusterEndpointType, error)
	DeleteClusterEndpointType(ctx context.Context, cetName string) error
	GetClusterEndpointTypeSchema(ctx context.Context, cetName string) (map[string]any, error)
}
