// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

import (
	"context"
	"log/slog"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateTrait = "trait:create"
	actionUpdateTrait = "trait:update"
	actionViewTrait   = "trait:view"
	actionDeleteTrait = "trait:delete"

	resourceTypeTrait = "trait"
)

// traitServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type traitServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*traitServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a trait service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &traitServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *traitServiceWithAuthz) CreateTrait(ctx context.Context, namespaceName string, t *openchoreov1alpha1.Trait) (*openchoreov1alpha1.Trait, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateTrait,
		ResourceType: resourceTypeTrait,
		ResourceID:   t.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateTrait(ctx, namespaceName, t)
}

func (s *traitServiceWithAuthz) UpdateTrait(ctx context.Context, namespaceName string, t *openchoreov1alpha1.Trait) (*openchoreov1alpha1.Trait, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateTrait,
		ResourceType: resourceTypeTrait,
		ResourceID:   t.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateTrait(ctx, namespaceName, t)
}

func (s *traitServiceWithAuthz) ListTraits(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Trait], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Trait], error) {
			return s.internal.ListTraits(ctx, namespaceName, pageOpts)
		},
		func(t openchoreov1alpha1.Trait) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewTrait,
				ResourceType: resourceTypeTrait,
				ResourceID:   t.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *traitServiceWithAuthz) GetTrait(ctx context.Context, namespaceName, traitName string) (*openchoreov1alpha1.Trait, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewTrait,
		ResourceType: resourceTypeTrait,
		ResourceID:   traitName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetTrait(ctx, namespaceName, traitName)
}

func (s *traitServiceWithAuthz) DeleteTrait(ctx context.Context, namespaceName, traitName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteTrait,
		ResourceType: resourceTypeTrait,
		ResourceID:   traitName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteTrait(ctx, namespaceName, traitName)
}

func (s *traitServiceWithAuthz) GetTraitSchema(ctx context.Context, namespaceName, traitName string) (*extv1.JSONSchemaProps, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewTrait,
		ResourceType: resourceTypeTrait,
		ResourceID:   traitName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetTraitSchema(ctx, namespaceName, traitName)
}
