// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
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

// CreateNamespace handles POST /api/v1/namespaces
func (h *Handler) CreateNamespace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request body
	var req models.CreateNamespaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to decode create namespace request", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	// Validate required fields
	if req.Name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	// Create namespace
	namespace, err := h.services.NamespaceService.CreateNamespace(ctx, &req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrNamespaceAlreadyExists) {
			writeErrorResponse(w, http.StatusConflict, "Namespace already exists", services.CodeNamespaceExists)
			return
		}
		h.logger.Error("Failed to create namespace", "error", err, "namespace", req.Name)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create namespace", services.CodeInternalError)
		return
	}

	h.logger.Info("Namespace created successfully", "namespace", req.Name)
	writeSuccessResponse(w, http.StatusCreated, namespace)
}
