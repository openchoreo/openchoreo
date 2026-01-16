// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// ListSecretReferences handles GET /api/v1/orgs/{namespaceName}/secret-references
func (h *Handler) ListSecretReferences(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	namespaceName := r.PathValue("namespaceName")

	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	secretReferences, err := h.services.SecretReferenceService.ListSecretReferences(ctx, namespaceName)
	if err != nil {
		if errors.Is(err, services.ErrNamespaceNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Namespace not found", services.CodeNamespaceNotFound)
			return
		}
		h.logger.Error("Failed to list secret references", "error", err, "org", namespaceName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list secret references", services.CodeInternalError)
		return
	}

	writeListResponse(w, secretReferences, len(secretReferences), 1, len(secretReferences))
}
