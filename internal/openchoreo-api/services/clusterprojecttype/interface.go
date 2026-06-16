// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterprojecttype

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the cluster project type service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	CreateClusterProjectType(ctx context.Context, cpt *openchoreov1alpha1.ClusterProjectType) (*openchoreov1alpha1.ClusterProjectType, error)
	UpdateClusterProjectType(ctx context.Context, cpt *openchoreov1alpha1.ClusterProjectType) (*openchoreov1alpha1.ClusterProjectType, error)
	ListClusterProjectTypes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterProjectType], error)
	GetClusterProjectType(ctx context.Context, cptName string) (*openchoreov1alpha1.ClusterProjectType, error)
	DeleteClusterProjectType(ctx context.Context, cptName string) error
	GetClusterProjectTypeSchema(ctx context.Context, cptName string) (map[string]any, error)
}
