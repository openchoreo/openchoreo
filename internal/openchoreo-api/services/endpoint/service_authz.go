// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	resourceTypeEndpoint = "endpoint"
)

// endpointServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
//
// Endpoint actions are evaluated at project scope: the authz hierarchy has no
// endpoint level, so the check names the owning project.
type endpointServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*endpointServiceWithAuthz)(nil)

// NewServiceWithAuthz creates an endpoint service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &endpointServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func checkRequest(action, namespaceName, projectName, endpointName string) services.CheckRequest {
	return services.CheckRequest{
		Action:       action,
		ResourceType: resourceTypeEndpoint,
		ResourceID:   endpointName,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   projectName,
		},
	}
}

func (s *endpointServiceWithAuthz) CreateEndpoint(ctx context.Context, namespaceName string, endpoint *openchoreov1alpha1.Endpoint) (*openchoreov1alpha1.Endpoint, error) {
	if endpoint == nil {
		return s.internal.CreateEndpoint(ctx, namespaceName, endpoint)
	}
	if err := s.authz.Check(ctx, checkRequest(authz.ActionCreateEndpoint,
		namespaceName, endpoint.Spec.Owner.ProjectName, endpoint.Name)); err != nil {
		return nil, err
	}
	return s.internal.CreateEndpoint(ctx, namespaceName, endpoint)
}

// UpdateEndpoint authorizes against the stored Endpoint's project, not the
// request body's, so a caller cannot gain access by naming a project they hold
// rights in. spec.owner is immutable, so the two only differ on a rejected update.
func (s *endpointServiceWithAuthz) UpdateEndpoint(ctx context.Context, namespaceName string, endpoint *openchoreov1alpha1.Endpoint) (*openchoreov1alpha1.Endpoint, error) {
	if endpoint == nil {
		return s.internal.UpdateEndpoint(ctx, namespaceName, endpoint)
	}
	existing, err := s.internal.GetEndpoint(ctx, namespaceName, endpoint.Name)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Check(ctx, checkRequest(authz.ActionUpdateEndpoint,
		namespaceName, existing.Spec.Owner.ProjectName, endpoint.Name)); err != nil {
		return nil, err
	}
	return s.internal.UpdateEndpoint(ctx, namespaceName, endpoint)
}

func (s *endpointServiceWithAuthz) ListEndpoints(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Endpoint], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Endpoint], error) {
			return s.internal.ListEndpoints(ctx, namespaceName, projectName, pageOpts)
		},
		func(ep openchoreov1alpha1.Endpoint) services.CheckRequest {
			return checkRequest(authz.ActionViewEndpoint, namespaceName, ep.Spec.Owner.ProjectName, ep.Name)
		},
	)
}

func (s *endpointServiceWithAuthz) GetEndpoint(ctx context.Context, namespaceName, endpointName string) (*openchoreov1alpha1.Endpoint, error) {
	// Fetch first to get the project for the authz hierarchy.
	ep, err := s.internal.GetEndpoint(ctx, namespaceName, endpointName)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Check(ctx, checkRequest(authz.ActionViewEndpoint,
		namespaceName, ep.Spec.Owner.ProjectName, endpointName)); err != nil {
		return nil, err
	}
	return ep, nil
}

func (s *endpointServiceWithAuthz) DeleteEndpoint(ctx context.Context, namespaceName, endpointName string) error {
	// Fetch first to get the project for the authz hierarchy.
	ep, err := s.internal.GetEndpoint(ctx, namespaceName, endpointName)
	if err != nil {
		return err
	}
	if err := s.authz.Check(ctx, checkRequest(authz.ActionDeleteEndpoint,
		namespaceName, ep.Spec.Owner.ProjectName, endpointName)); err != nil {
		return err
	}
	return s.internal.DeleteEndpoint(ctx, namespaceName, endpointName)
}
