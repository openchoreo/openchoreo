// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

type ListOrganizationsResponse struct {
	Organizations []*models.OrganizationResponse `json:"organizations"`
}

func (h *MCPHandler) GetOrganization(ctx context.Context, name string) (any, error) {
	return h.getOrganizationByName(ctx, name)
}

func (h *MCPHandler) ListOrganizations(ctx context.Context) (any, error) {
	return h.listOrganizations(ctx)
}

func (h *MCPHandler) listOrganizations(ctx context.Context) (ListOrganizationsResponse, error) {
	organizations, err := h.Services.OrganizationService.ListOrganizations(ctx)
	if err != nil {
		return ListOrganizationsResponse{}, err
	}
	return ListOrganizationsResponse{
		Organizations: organizations,
	}, nil
}

func (h *MCPHandler) getOrganizationByName(ctx context.Context, name string) (*models.OrganizationResponse, error) {
	return h.Services.OrganizationService.GetOrganization(ctx, name)
}

type ListSecretReferencesResponse struct {
	SecretReferences []*models.SecretReferenceResponse `json:"secret_references"`
}

func (h *MCPHandler) ListSecretReferences(ctx context.Context, orgName string) (any, error) {
	secretReferences, err := h.Services.SecretReferenceService.ListSecretReferences(ctx, orgName)
	if err != nil {
		return ListSecretReferencesResponse{}, err
	}
	return ListSecretReferencesResponse{
		SecretReferences: secretReferences,
	}, nil
}
