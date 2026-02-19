// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateComponentType = "componenttype:create"
	actionUpdateComponentType = "componenttype:update"
	actionViewComponentType   = "componenttype:view"
	actionDeleteComponentType = "componenttype:delete"

	resourceTypeComponentType = "componenttype"
)

// componentTypeServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type componentTypeServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*componentTypeServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a component type service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &componentTypeServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *componentTypeServiceWithAuthz) CreateComponentType(ctx context.Context, namespaceName string, ct *openchoreov1alpha1.ComponentType) (*openchoreov1alpha1.ComponentType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateComponentType,
		ResourceType: resourceTypeComponentType,
		ResourceID:   ct.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateComponentType(ctx, namespaceName, ct)
}

func (s *componentTypeServiceWithAuthz) UpdateComponentType(ctx context.Context, namespaceName string, ct *openchoreov1alpha1.ComponentType) (*openchoreov1alpha1.ComponentType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateComponentType,
		ResourceType: resourceTypeComponentType,
		ResourceID:   ct.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateComponentType(ctx, namespaceName, ct)
}

func (s *componentTypeServiceWithAuthz) ListComponentTypes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ComponentType], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ComponentType], error) {
			return s.internal.ListComponentTypes(ctx, namespaceName, pageOpts)
		},
		func(ct openchoreov1alpha1.ComponentType) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewComponentType,
				ResourceType: resourceTypeComponentType,
				ResourceID:   ct.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *componentTypeServiceWithAuthz) GetComponentType(ctx context.Context, namespaceName, ctName string) (*openchoreov1alpha1.ComponentType, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewComponentType,
		ResourceType: resourceTypeComponentType,
		ResourceID:   ctName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetComponentType(ctx, namespaceName, ctName)
}

func (s *componentTypeServiceWithAuthz) DeleteComponentType(ctx context.Context, namespaceName, ctName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteComponentType,
		ResourceType: resourceTypeComponentType,
		ResourceID:   ctName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteComponentType(ctx, namespaceName, ctName)
}
