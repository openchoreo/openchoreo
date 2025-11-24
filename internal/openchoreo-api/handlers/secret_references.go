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

	secretReferences, err := h.services.SecretReferenceService.ListSecretReferences(ctx, orgName)
	if err != nil {
		if errors.Is(err, services.ErrOrganizationNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Organization not found", services.CodeOrganizationNotFound)
			return
		}
		h.logger.Error("Failed to list secret references", "error", err, "org", orgName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list secret references", services.CodeInternalError)
		return
	}

	writeListResponse(w, secretReferences, len(secretReferences), 1, len(secretReferences))
}
