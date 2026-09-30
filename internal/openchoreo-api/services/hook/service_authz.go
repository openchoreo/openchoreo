// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hook

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeHook = "hook"
)

// hookServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type hookServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*hookServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a hook service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &hookServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *hookServiceWithAuthz) CreateHook(ctx context.Context, namespaceName string, h *openchoreov1alpha1.Hook) (*openchoreov1alpha1.Hook, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateHook,
		ResourceType: resourceTypeHook,
		ResourceID:   h.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateHook(ctx, namespaceName, h)
}

func (s *hookServiceWithAuthz) UpdateHook(ctx context.Context, namespaceName string, h *openchoreov1alpha1.Hook) (*openchoreov1alpha1.Hook, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateHook,
		ResourceType: resourceTypeHook,
		ResourceID:   h.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateHook(ctx, namespaceName, h)
}

func (s *hookServiceWithAuthz) ListHooks(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Hook], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Hook], error) {
			return s.internal.ListHooks(ctx, namespaceName, pageOpts)
		},
		func(h openchoreov1alpha1.Hook) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewHook,
				ResourceType: resourceTypeHook,
				ResourceID:   h.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *hookServiceWithAuthz) GetHook(ctx context.Context, namespaceName, hookName string) (*openchoreov1alpha1.Hook, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewHook,
		ResourceType: resourceTypeHook,
		ResourceID:   hookName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetHook(ctx, namespaceName, hookName)
}

func (s *hookServiceWithAuthz) DeleteHook(ctx context.Context, namespaceName, hookName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteHook,
		ResourceType: resourceTypeHook,
		ResourceID:   hookName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteHook(ctx, namespaceName, hookName)
}
