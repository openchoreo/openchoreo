// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// ListOrganizations returns a paginated list of organizations
func (h *Handler) ListOrganizations(
	ctx context.Context,
	request gen.ListOrganizationsRequestObject,
) (gen.ListOrganizationsResponseObject, error) {
	h.logger.Debug("ListOrganizations called")

	orgs, err := h.services.OrganizationService.ListOrganizations(ctx)
	if err != nil {
		h.logger.Error("Failed to list organizations", "error", err)
		return gen.ListOrganizations500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.Organization, 0, len(orgs))
	for _, org := range orgs {
		items = append(items, toGenOrganization(org))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListOrganizations200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// GetOrganization returns details of a specific organization
func (h *Handler) GetOrganization(
	ctx context.Context,
	request gen.GetOrganizationRequestObject,
) (gen.GetOrganizationResponseObject, error) {
	h.logger.Debug("GetOrganization called", "orgName", request.OrgName)

	org, err := h.services.OrganizationService.GetOrganization(ctx, request.OrgName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetOrganization403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrOrganizationNotFound) {
			return gen.GetOrganization404JSONResponse{NotFoundJSONResponse: notFound("Organization")}, nil
		}
		h.logger.Error("Failed to get organization", "error", err)
		return gen.GetOrganization500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetOrganization200JSONResponse(toGenOrganization(org)), nil
}

// toGenOrganization converts models.OrganizationResponse to gen.Organization
func toGenOrganization(org *models.OrganizationResponse) gen.Organization {
	return gen.Organization{
		Name:        org.Name,
		DisplayName: ptr.To(org.DisplayName),
		Description: ptr.To(org.Description),
		Namespace:   ptr.To(org.Namespace),
		CreatedAt:   org.CreatedAt,
		Status:      ptr.To(org.Status),
	}
}
