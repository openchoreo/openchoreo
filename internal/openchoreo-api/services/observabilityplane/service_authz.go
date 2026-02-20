// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionViewObservabilityPlane = "observabilityplane:view"

	resourceTypeObservabilityPlane = "observabilityplane"
)

// observabilityPlaneServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type observabilityPlaneServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*observabilityPlaneServiceWithAuthz)(nil)

// NewServiceWithAuthz creates an observability plane service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &observabilityPlaneServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *observabilityPlaneServiceWithAuthz) ListObservabilityPlanes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ObservabilityPlane], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ObservabilityPlane], error) {
			return s.internal.ListObservabilityPlanes(ctx, namespaceName, pageOpts)
		},
		func(op openchoreov1alpha1.ObservabilityPlane) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewObservabilityPlane,
				ResourceType: resourceTypeObservabilityPlane,
				ResourceID:   op.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *observabilityPlaneServiceWithAuthz) GetObservabilityPlane(ctx context.Context, namespaceName, observabilityPlaneName string) (*openchoreov1alpha1.ObservabilityPlane, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewObservabilityPlane,
		ResourceType: resourceTypeObservabilityPlane,
		ResourceID:   observabilityPlaneName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetObservabilityPlane(ctx, namespaceName, observabilityPlaneName)
}
