// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

type ListNamespacesResponse struct {
	Namespaces []*models.NamespaceResponse `json:"namespaces"`
}

func (h *MCPHandler) GetNamespace(ctx context.Context, name string) (any, error) {
	return h.getNamespaceByName(ctx, name)
}

func (h *MCPHandler) ListNamespaces(ctx context.Context) (any, error) {
	return h.listNamespaces(ctx)
}

func (h *MCPHandler) listNamespaces(ctx context.Context) (ListNamespacesResponse, error) {
	namespaces, err := h.Services.NamespaceService.ListNamespaces(ctx)
	if err != nil {
		return ListNamespacesResponse{}, err
	}
	return ListNamespacesResponse{
		Namespaces: namespaces,
	}, nil
}

func (h *MCPHandler) getNamespaceByName(ctx context.Context, name string) (*models.NamespaceResponse, error) {
	return h.Services.NamespaceService.GetNamespace(ctx, name)
}

type ListSecretReferencesResponse struct {
	SecretReferences []*models.SecretReferenceResponse `json:"secret_references"`
}

func (h *MCPHandler) ListSecretReferences(ctx context.Context, namespaceName string) (any, error) {
	secretReferences, err := h.Services.SecretReferenceService.ListSecretReferences(ctx, namespaceName)
	if err != nil {
		return ListSecretReferencesResponse{}, err
	}
	return ListSecretReferencesResponse{
		SecretReferences: secretReferences,
	}, nil
}
