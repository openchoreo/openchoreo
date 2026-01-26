// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// ListOrganizations handles GET /api/v1/orgs
func (h *Handler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract and validate list parameters
	opts, err := extractListParams(r.URL.Query())
	if err != nil {
		h.logger.Warn("Invalid list parameters", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	result, err := h.services.OrganizationService.ListOrganizations(ctx, opts)
	if err != nil {
		if errors.Is(err, services.ErrContinueTokenExpired) {
			h.logger.Warn("Continue token expired")
			writeErrorResponse(w, http.StatusGone,
				"Continue token has expired. Please restart listing from the beginning.",
				services.CodeContinueTokenExpired)
			return
		}
		if errors.Is(err, services.ErrInvalidContinueToken) {
			h.logger.Warn("Invalid continue token provided")
			writeErrorResponse(w, http.StatusBadRequest,
				"Invalid continue token provided",
				services.CodeInvalidContinueToken)
			return
		}
		h.logger.Error("Failed to list organizations", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list organizations", services.CodeInternalError)
		return
	}

	h.logger.Debug("Listed organizations successfully", "count", len(result.Items), "hasMore", result.Metadata.HasMore)
	writeListResponse(w, result.Items, result.Metadata.ResourceVersion, result.Metadata.Continue)
}

// GetOrganization handles GET /api/v1/orgs/{orgName}
func (h *Handler) GetOrganization(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName := r.PathValue("orgName")

	if orgName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	organization, err := h.services.OrganizationService.GetOrganization(ctx, orgName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrOrganizationNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Organization not found", services.CodeOrganizationNotFound)
			return
		}
		h.logger.Error("Failed to get organization", "error", err, "org", orgName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get organization", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, organization)
}
