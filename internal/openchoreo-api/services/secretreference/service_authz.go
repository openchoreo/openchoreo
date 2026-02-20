// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secretreference

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateSecretReference = "secretreference:create"
	actionUpdateSecretReference = "secretreference:update"
	actionViewSecretReference   = "secretreference:view"
	actionDeleteSecretReference = "secretreference:delete"

	resourceTypeSecretReference = "secretReference"
)

// secretReferenceServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type secretReferenceServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*secretReferenceServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a secret reference service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &secretReferenceServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *secretReferenceServiceWithAuthz) CreateSecretReference(ctx context.Context, namespaceName string, sr *openchoreov1alpha1.SecretReference) (*openchoreov1alpha1.SecretReference, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateSecretReference,
		ResourceType: resourceTypeSecretReference,
		ResourceID:   sr.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateSecretReference(ctx, namespaceName, sr)
}

func (s *secretReferenceServiceWithAuthz) UpdateSecretReference(ctx context.Context, namespaceName string, sr *openchoreov1alpha1.SecretReference) (*openchoreov1alpha1.SecretReference, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateSecretReference,
		ResourceType: resourceTypeSecretReference,
		ResourceID:   sr.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateSecretReference(ctx, namespaceName, sr)
}

func (s *secretReferenceServiceWithAuthz) ListSecretReferences(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.SecretReference], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.SecretReference], error) {
			return s.internal.ListSecretReferences(ctx, namespaceName, pageOpts)
		},
		func(sr openchoreov1alpha1.SecretReference) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewSecretReference,
				ResourceType: resourceTypeSecretReference,
				ResourceID:   sr.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *secretReferenceServiceWithAuthz) GetSecretReference(ctx context.Context, namespaceName, secretReferenceName string) (*openchoreov1alpha1.SecretReference, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewSecretReference,
		ResourceType: resourceTypeSecretReference,
		ResourceID:   secretReferenceName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetSecretReference(ctx, namespaceName, secretReferenceName)
}

func (s *secretReferenceServiceWithAuthz) DeleteSecretReference(ctx context.Context, namespaceName, secretReferenceName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteSecretReference,
		ResourceType: resourceTypeSecretReference,
		ResourceID:   secretReferenceName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteSecretReference(ctx, namespaceName, secretReferenceName)
}
