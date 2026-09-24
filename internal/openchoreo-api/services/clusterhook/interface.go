// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterhook

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the cluster hook service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
type Service interface {
	CreateClusterHook(ctx context.Context, ch *openchoreov1alpha1.ClusterHook) (*openchoreov1alpha1.ClusterHook, error)
	UpdateClusterHook(ctx context.Context, ch *openchoreov1alpha1.ClusterHook) (*openchoreov1alpha1.ClusterHook, error)
	ListClusterHooks(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterHook], error)
	GetClusterHook(ctx context.Context, clusterHookName string) (*openchoreov1alpha1.ClusterHook, error)
	DeleteClusterHook(ctx context.Context, clusterHookName string) error
}
