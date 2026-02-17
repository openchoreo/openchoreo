// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacy_services"
)

// ListDataPlanes handles GET /api/v1/namespaces/{namespaceName}/dataplanes
func (h *Handler) ListDataPlanes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	namespaceName := r.PathValue("namespaceName")

	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}
	dataplanes, err := h.services.DataPlaneService.ListDataPlanes(ctx, namespaceName)
	if err != nil {
		h.logger.Error("Failed to list dataplanes", "error", err, "namespace", namespaceName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list dataplanes", services.CodeInternalError)
		return
	}

	writeListResponse(w, dataplanes, len(dataplanes), 1, len(dataplanes))
}

// GetDataPlane handles GET /api/v1/namespaces/{namespaceName}/dataplanes/{dpName}
func (h *Handler) GetDataPlane(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	namespaceName := r.PathValue("namespaceName")
	dpName := r.PathValue("dpName")

	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	if dpName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "DataPlane name is required", services.CodeInvalidInput)
		return
	}

	dataplane, err := h.services.DataPlaneService.GetDataPlane(ctx, namespaceName, dpName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to view dataplane", "namespace", namespaceName, "dataplane", dpName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrDataPlaneNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "DataPlane not found", services.CodeDataPlaneNotFound)
			return
		}
		h.logger.Error("Failed to get dataplane", "error", err, "namespace", namespaceName, "dataplane", dpName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get dataplane", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, dataplane)
}

// CreateDataPlane handles POST /api/v1/namespaces/{namespaceName}/dataplanes
func (h *Handler) CreateDataPlane(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	namespaceName := r.PathValue("namespaceName")

	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	var req models.CreateDataPlaneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to decode request body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		h.logger.Error("Request validation failed", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request data", services.CodeInvalidInput)
		return
	}

	setAuditResource(ctx, "dataplane", req.Name, req.Name)
	addAuditMetadata(ctx, "organization", namespaceName)

	dataplane, err := h.services.DataPlaneService.CreateDataPlane(ctx, namespaceName, &req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to create dataplane", "namespace", namespaceName, "dataplane", req.Name)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrDataPlaneAlreadyExists) {
			writeErrorResponse(w, http.StatusConflict, "DataPlane already exists", services.CodeDataPlaneExists)
			return
		}
		h.logger.Error("Failed to create dataplane", "error", err, "namespace", namespaceName, "dataplane", req.Name)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create dataplane", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusCreated, dataplane)
}
