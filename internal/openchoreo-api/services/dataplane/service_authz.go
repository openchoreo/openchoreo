// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionViewDataPlane   = "dataplane:view"
	actionCreateDataPlane = "dataplane:create"
	actionUpdateDataPlane = "dataplane:update"
	actionDeleteDataPlane = "dataplane:delete"

	resourceTypeDataPlane = "dataPlane"
)

// dataPlaneServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type dataPlaneServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*dataPlaneServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a data plane service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &dataPlaneServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *dataPlaneServiceWithAuthz) ListDataPlanes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.DataPlane], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.DataPlane], error) {
			return s.internal.ListDataPlanes(ctx, namespaceName, pageOpts)
		},
		func(dp openchoreov1alpha1.DataPlane) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewDataPlane,
				ResourceType: resourceTypeDataPlane,
				ResourceID:   dp.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *dataPlaneServiceWithAuthz) GetDataPlane(ctx context.Context, namespaceName, dpName string) (*openchoreov1alpha1.DataPlane, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewDataPlane,
		ResourceType: resourceTypeDataPlane,
		ResourceID:   dpName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetDataPlane(ctx, namespaceName, dpName)
}

func (s *dataPlaneServiceWithAuthz) CreateDataPlane(ctx context.Context, namespaceName string, dp *openchoreov1alpha1.DataPlane) (*openchoreov1alpha1.DataPlane, error) {
	if dp == nil {
		return nil, ErrDataPlaneNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateDataPlane,
		ResourceType: resourceTypeDataPlane,
		ResourceID:   dp.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateDataPlane(ctx, namespaceName, dp)
}

func (s *dataPlaneServiceWithAuthz) UpdateDataPlane(ctx context.Context, namespaceName string, dp *openchoreov1alpha1.DataPlane) (*openchoreov1alpha1.DataPlane, error) {
	if dp == nil {
		return nil, ErrDataPlaneNil
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateDataPlane,
		ResourceType: resourceTypeDataPlane,
		ResourceID:   dp.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateDataPlane(ctx, namespaceName, dp)
}

func (s *dataPlaneServiceWithAuthz) DeleteDataPlane(ctx context.Context, namespaceName, dpName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteDataPlane,
		ResourceType: resourceTypeDataPlane,
		ResourceID:   dpName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteDataPlane(ctx, namespaceName, dpName)
}
