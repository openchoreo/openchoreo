// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// ListNamespaces handles GET /api/v1/namespaces
func (h *Handler) ListNamespaces(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	namespaces, err := h.services.NamespaceService.ListNamespaces(ctx)
	if err != nil {
		h.logger.Error("Failed to list namespaces", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list namespaces", services.CodeInternalError)
		return
	}

	writeListResponse(w, namespaces, len(namespaces), 1, len(namespaces))
}

// GetNamespace handles GET /api/v1/namespaces/{namespaceName}
func (h *Handler) GetNamespace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	namespaceName := r.PathValue("namespaceName")

	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	namespace, err := h.services.NamespaceService.GetNamespace(ctx, namespaceName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrNamespaceNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Namespace not found", services.CodeNamespaceNotFound)
			return
		}
		h.logger.Error("Failed to get namespace", "error", err, "namespace", namespaceName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get namespace", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, namespace)
}
