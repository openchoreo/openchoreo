// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
)

func (h *MCPHandler) GetOrganization(ctx context.Context, name string) (string, error) {
	if name == "" {
		return h.listOrganizations(ctx)
	} else {
		return h.getOrganizationByName(ctx, name)
	}
}

func (h *MCPHandler) listOrganizations(ctx context.Context) (string, error) {
	res, err := h.Services.OrganizationService.ListOrganizations(ctx)
	if err != nil {
		return "", err
	}
	return marshalResponse(res)
}

func (h *MCPHandler) getOrganizationByName(ctx context.Context, name string) (string, error) {
	res, err := h.Services.OrganizationService.GetOrganization(ctx, name)
	if err != nil {
		return "", err
	}
	return marshalResponse(res)
}
