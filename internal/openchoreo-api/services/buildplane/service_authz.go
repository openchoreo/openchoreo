// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionViewBuildPlane = "buildplane:view"

	resourceTypeBuildPlane = "buildplane"
)

// buildPlaneServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type buildPlaneServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*buildPlaneServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a build plane service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &buildPlaneServiceWithAuthz{
		internal: NewService(k8sClient, nil, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *buildPlaneServiceWithAuthz) ListBuildPlanes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.BuildPlane], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.BuildPlane], error) {
			return s.internal.ListBuildPlanes(ctx, namespaceName, pageOpts)
		},
		func(bp openchoreov1alpha1.BuildPlane) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewBuildPlane,
				ResourceType: resourceTypeBuildPlane,
				ResourceID:   bp.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *buildPlaneServiceWithAuthz) GetBuildPlane(ctx context.Context, namespaceName, buildPlaneName string) (*openchoreov1alpha1.BuildPlane, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewBuildPlane,
		ResourceType: resourceTypeBuildPlane,
		ResourceID:   buildPlaneName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetBuildPlane(ctx, namespaceName, buildPlaneName)
}

// GetBuildPlaneClient is not implemented on the authz-wrapped service as it is not exposed externally.
func (s *buildPlaneServiceWithAuthz) GetBuildPlaneClient(_ context.Context, _, _ string) (client.Client, error) {
	return nil, errNotImplemented
}

// ArgoWorkflowExists is not implemented on the authz-wrapped service as it is not exposed externally.
func (s *buildPlaneServiceWithAuthz) ArgoWorkflowExists(_ context.Context, _, _ string, _ *openchoreov1alpha1.ResourceReference) bool {
	return false
}
