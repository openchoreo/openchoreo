// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

import (
	"context"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the cluster trait service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	CreateClusterTrait(ctx context.Context, ct *openchoreov1alpha1.ClusterTrait) (*openchoreov1alpha1.ClusterTrait, error)
	UpdateClusterTrait(ctx context.Context, ct *openchoreov1alpha1.ClusterTrait) (*openchoreov1alpha1.ClusterTrait, error)
	ListClusterTraits(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterTrait], error)
	GetClusterTrait(ctx context.Context, clusterTraitName string) (*openchoreov1alpha1.ClusterTrait, error)
	DeleteClusterTrait(ctx context.Context, clusterTraitName string) error
	GetClusterTraitSchema(ctx context.Context, clusterTraitName string) (*extv1.JSONSchemaProps, error)
}
