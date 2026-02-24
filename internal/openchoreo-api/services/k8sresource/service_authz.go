// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresource

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionViewDataPlaneK8sResource          = "dataplanek8sresource:view"
	actionViewBuildPlaneK8sResource         = "buildplanek8sresource:view"
	actionViewObservabilityPlaneK8sResource = "observabilityplanek8sresource:view"

	actionViewProject     = "project:view"
	actionViewComponent   = "component:view"
	actionViewEnvironment = "environment:view"

	resourceTypeDataPlaneK8sResource          = "dataPlaneK8sResource"
	resourceTypeBuildPlaneK8sResource         = "buildPlaneK8sResource"
	resourceTypeObservabilityPlaneK8sResource = "observabilityPlaneK8sResource"

	resourceTypeProject     = "project"
	resourceTypeComponent   = "component"
	resourceTypeEnvironment = "environment"
)

// k8sResourceServiceWithAuthz wraps the core service with authorization checks.
type k8sResourceServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*k8sResourceServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a k8s resource query service with authorization.
func NewServiceWithAuthz(k8sClient client.Client, gatewayClient *gateway.Client, pdp authz.PDP, logger *slog.Logger) Service {
	return &k8sResourceServiceWithAuthz{
		internal: NewService(k8sClient, gatewayClient, logger),
		authz:    services.NewAuthzChecker(pdp, logger),
	}
}

func (s *k8sResourceServiceWithAuthz) QueryK8sResources(ctx context.Context, req *QueryRequest) (*QueryResponse, error) {
	// Validate mandatory fields: project and component are required for all planes.
	if req.Project == "" {
		return nil, fmt.Errorf("%w: project is required", ErrMissingRequired)
	}
	if req.Component == "" {
		return nil, fmt.Errorf("%w: component is required", ErrMissingRequired)
	}
	// Environment is required for dataplane and observabilityplane (buildplane is not bound to an environment).
	if req.PlaneType != planeTypeBuildPlane && req.Environment == "" {
		return nil, fmt.Errorf("%w: environment is required for %s", ErrMissingRequired, req.PlaneType)
	}

	action, resType := planeTypeToAuthz(req.PlaneType)

	hierarchy := authz.ResourceHierarchy{
		Project:     req.Project,
		Component:   req.Component,
		Environment: req.Environment,
	}
	if req.PlaneNamespace != "" {
		hierarchy.Namespace = req.PlaneNamespace
	}

	// Check plane k8s resource permission
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       action,
		ResourceType: resType,
		ResourceID:   req.PlaneName,
		Hierarchy:    hierarchy,
	}); err != nil {
		return nil, err
	}

	// Verify the user has read access to the referenced project, component,
	// and environment.
	if err := s.checkResourceAccess(ctx, req); err != nil {
		return nil, err
	}

	return s.internal.QueryK8sResources(ctx, req)
}

// checkResourceAccess verifies the user has view permissions on the project,
// component, and environment provided in the request.
func (s *k8sResourceServiceWithAuthz) checkResourceAccess(ctx context.Context, req *QueryRequest) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewProject,
		ResourceType: resourceTypeProject,
		ResourceID:   req.Project,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: req.PlaneNamespace,
			Project:   req.Project,
		},
	}); err != nil {
		return err
	}

	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewComponent,
		ResourceType: resourceTypeComponent,
		ResourceID:   req.Component,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: req.PlaneNamespace,
			Project:   req.Project,
			Component: req.Component,
		},
	}); err != nil {
		return err
	}

	if req.Environment != "" {
		if err := s.authz.Check(ctx, services.CheckRequest{
			Action:       actionViewEnvironment,
			ResourceType: resourceTypeEnvironment,
			ResourceID:   req.Environment,
			Hierarchy: authz.ResourceHierarchy{
				Namespace: req.PlaneNamespace,
			},
		}); err != nil {
			return err
		}
	}

	return nil
}

func planeTypeToAuthz(planeType string) (action, resourceType string) {
	switch planeType {
	case planeTypeBuildPlane:
		return actionViewBuildPlaneK8sResource, resourceTypeBuildPlaneK8sResource
	case planeTypeObservabilityPlane:
		return actionViewObservabilityPlaneK8sResource, resourceTypeObservabilityPlaneK8sResource
	default: // dataplane
		return actionViewDataPlaneK8sResource, resourceTypeDataPlaneK8sResource
	}
}
