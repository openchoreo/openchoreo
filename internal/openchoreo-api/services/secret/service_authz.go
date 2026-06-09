// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const resourceTypeSecret = "secret"

type secretServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

// NewServiceWithAuthz creates a new secret service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, planeClientProvider kubernetesClient.PlaneClientProvider, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &secretServiceWithAuthz{
		internal: NewService(k8sClient, planeClientProvider, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *secretServiceWithAuthz) CreateSecret(ctx context.Context, namespaceName string, req *CreateSecretParams) (*corev1.Secret, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionCreateSecret,
		ResourceType: resourceTypeSecret,
		ResourceID:   req.SecretName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateSecret(ctx, namespaceName, req)
}

func (s *secretServiceWithAuthz) UpdateSecret(ctx context.Context, namespaceName, secretName string, req *UpdateSecretParams) (*corev1.Secret, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionUpdateSecret,
		ResourceType: resourceTypeSecret,
		ResourceID:   secretName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateSecret(ctx, namespaceName, secretName, req)
}

func (s *secretServiceWithAuthz) GetSecret(ctx context.Context, namespaceName, secretName string) (*corev1.Secret, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionViewSecret,
		ResourceType: resourceTypeSecret,
		ResourceID:   secretName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetSecret(ctx, namespaceName, secretName)
}

func (s *secretServiceWithAuthz) ListSecrets(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[corev1.Secret], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[corev1.Secret], error) {
			return s.internal.ListSecrets(ctx, namespaceName, pageOpts)
		},
		func(secret corev1.Secret) services.CheckRequest {
			return services.CheckRequest{
				Action:       authz.ActionViewSecret,
				ResourceType: resourceTypeSecret,
				ResourceID:   secret.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *secretServiceWithAuthz) DeleteSecret(ctx context.Context, namespaceName, secretName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionDeleteSecret,
		ResourceType: resourceTypeSecret,
		ResourceID:   secretName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteSecret(ctx, namespaceName, secretName)
}
