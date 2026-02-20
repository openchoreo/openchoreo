// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionViewEnvironment   = "environment:view"
	actionCreateEnvironment = "environment:create"

	resourceTypeEnvironment = "environment"
)

// environmentServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type environmentServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*environmentServiceWithAuthz)(nil)

// NewServiceWithAuthz creates an environment service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &environmentServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *environmentServiceWithAuthz) ListEnvironments(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Environment], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Environment], error) {
			return s.internal.ListEnvironments(ctx, namespaceName, pageOpts)
		},
		func(env openchoreov1alpha1.Environment) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewEnvironment,
				ResourceType: resourceTypeEnvironment,
				ResourceID:   env.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *environmentServiceWithAuthz) GetEnvironment(ctx context.Context, namespaceName, envName string) (*openchoreov1alpha1.Environment, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewEnvironment,
		ResourceType: resourceTypeEnvironment,
		ResourceID:   envName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetEnvironment(ctx, namespaceName, envName)
}

func (s *environmentServiceWithAuthz) CreateEnvironment(ctx context.Context, namespaceName string, env *openchoreov1alpha1.Environment) (*openchoreov1alpha1.Environment, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateEnvironment,
		ResourceType: resourceTypeEnvironment,
		ResourceID:   env.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateEnvironment(ctx, namespaceName, env)
}

func (s *environmentServiceWithAuthz) GetObserverURL(ctx context.Context, namespaceName, envName string) (*ObserverURLResult, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewEnvironment,
		ResourceType: resourceTypeEnvironment,
		ResourceID:   envName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetObserverURL(ctx, namespaceName, envName)
}

func (s *environmentServiceWithAuthz) GetRCAAgentURL(ctx context.Context, namespaceName, envName string) (*RCAAgentURLResult, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewEnvironment,
		ResourceType: resourceTypeEnvironment,
		ResourceID:   envName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetRCAAgentURL(ctx, namespaceName, envName)
}
