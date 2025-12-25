// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// ListSecretReferences handles GET /api/v1/orgs/{orgName}/secret-references
func (h *Handler) ListSecretReferences(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName := r.PathValue("orgName")

	if orgName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	// Extract and validate list parameters
	opts, err := extractListParams(r.URL.Query())
	if err != nil {
		h.logger.Warn("Invalid list parameters", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	result, err := h.services.SecretReferenceService.ListSecretReferences(ctx, orgName, opts)
	if err != nil {
		if errors.Is(err, services.ErrOrganizationNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Organization not found", services.CodeOrganizationNotFound)
			return
		}
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
		h.logger.Error("Failed to list secret references", "error", err, "org", orgName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list secret references", services.CodeInternalError)
		return
	}

	writeListResponse(w, result.Items, result.Metadata.ResourceVersion, result.Metadata.Continue)
}
