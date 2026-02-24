// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gitsecret

import (
	"context"
	"errors"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateSecretReference = "secretreference:create"
	actionViewSecretReference   = "secretreference:view"
	actionDeleteSecretReference = "secretreference:delete"

	resourceTypeSecretReference = "secretReference"
)

type gitSecretServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

// NewServiceWithAuthz creates a new git secret service with authorization.
func NewServiceWithAuthz(k8sClient client.Client, bpClientMgr *kubernetesClient.KubeMultiClientManager, authzPDP authz.PDP, logger *slog.Logger, gatewayURL string) Service {
	return &gitSecretServiceWithAuthz{
		internal: NewService(k8sClient, bpClientMgr, logger, gatewayURL),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

// ListGitSecrets returns git secrets the caller is authorized to view.
func (s *gitSecretServiceWithAuthz) ListGitSecrets(ctx context.Context, namespaceName string) ([]GitSecretInfo, error) {
	items, err := s.internal.ListGitSecrets(ctx, namespaceName)
	if err != nil {
		return nil, err
	}

	authorized := make([]GitSecretInfo, 0, len(items))
	for _, item := range items {
		if err := s.authz.Check(ctx, services.CheckRequest{
			Action:       actionViewSecretReference,
			ResourceType: resourceTypeSecretReference,
			ResourceID:   item.Name,
			Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
		}); err != nil {
			if errors.Is(err, services.ErrForbidden) {
				continue
			}
			return nil, err
		}
		authorized = append(authorized, item)
	}

	return authorized, nil
}

// CreateGitSecret creates a git secret after checking authorization.
func (s *gitSecretServiceWithAuthz) CreateGitSecret(ctx context.Context, namespaceName string, req *CreateGitSecretParams) (*GitSecretInfo, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateSecretReference,
		ResourceType: resourceTypeSecretReference,
		ResourceID:   req.SecretName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateGitSecret(ctx, namespaceName, req)
}

// DeleteGitSecret deletes a git secret after checking authorization.
func (s *gitSecretServiceWithAuthz) DeleteGitSecret(ctx context.Context, namespaceName, secretName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteSecretReference,
		ResourceType: resourceTypeSecretReference,
		ResourceID:   secretName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteGitSecret(ctx, namespaceName, secretName)
}
