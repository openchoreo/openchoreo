// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterhook

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeClusterHook = "clusterHook"
)

// clusterHookServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type clusterHookServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*clusterHookServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a cluster hook service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &clusterHookServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *clusterHookServiceWithAuthz) CreateClusterHook(ctx context.Context, ch *openchoreov1alpha1.ClusterHook) (*openchoreov1alpha1.ClusterHook, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateClusterHook,
		ResourceType: resourceTypeClusterHook,
		ResourceID:   ch.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateClusterHook(ctx, ch)
}

func (s *clusterHookServiceWithAuthz) UpdateClusterHook(ctx context.Context, ch *openchoreov1alpha1.ClusterHook) (*openchoreov1alpha1.ClusterHook, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateClusterHook,
		ResourceType: resourceTypeClusterHook,
		ResourceID:   ch.Name,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateClusterHook(ctx, ch)
}

func (s *clusterHookServiceWithAuthz) ListClusterHooks(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterHook], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterHook], error) {
			return s.internal.ListClusterHooks(ctx, pageOpts)
		},
		func(ch openchoreov1alpha1.ClusterHook) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewClusterHook,
				ResourceType: resourceTypeClusterHook,
				ResourceID:   ch.Name,
				Hierarchy:    authz.ResourceHierarchy{},
			}
		},
	)
}

func (s *clusterHookServiceWithAuthz) GetClusterHook(ctx context.Context, clusterHookName string) (*openchoreov1alpha1.ClusterHook, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewClusterHook,
		ResourceType: resourceTypeClusterHook,
		ResourceID:   clusterHookName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetClusterHook(ctx, clusterHookName)
}

// DeleteClusterHook checks delete authorization before delegating to the internal service.
func (s *clusterHookServiceWithAuthz) DeleteClusterHook(ctx context.Context, clusterHookName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteClusterHook,
		ResourceType: resourceTypeClusterHook,
		ResourceID:   clusterHookName,
		Hierarchy:    authz.ResourceHierarchy{},
	}); err != nil {
		return err
	}
	return s.internal.DeleteClusterHook(ctx, clusterHookName)
}
