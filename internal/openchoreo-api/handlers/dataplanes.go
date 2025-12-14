// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// ListDataPlanes handles GET /api/v1/orgs/{orgName}/dataplanes
func (h *Handler) ListDataPlanes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName := r.PathValue("orgName")

	if orgName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	opts, err := extractListParams(r.URL.Query())
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	result, err := h.services.DataPlaneService.ListDataPlanes(ctx, orgName, opts)
	if err != nil {
		if errors.Is(err, services.ErrContinueTokenExpired) {
			writeErrorResponse(w, http.StatusGone, "Continue token has expired, please restart listing", services.CodeContinueTokenExpired)
			return
		}
		if errors.Is(err, services.ErrInvalidContinueToken) {
			writeErrorResponse(w, http.StatusBadRequest, "Invalid continue token", services.CodeInvalidContinueToken)
			return
		}
		h.logger.Error("Failed to list dataplanes", "error", err, "org", orgName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list dataplanes", services.CodeInternalError)
		return
	}

	writeListResponse(w, result.Items, result.Metadata.ResourceVersion, result.Metadata.Continue)
}

// GetDataPlane handles GET /api/v1/orgs/{orgName}/dataplanes/{dpName}
func (h *Handler) GetDataPlane(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName := r.PathValue("orgName")
	dpName := r.PathValue("dpName")

	if orgName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	if dpName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "DataPlane name is required", services.CodeInvalidInput)
		return
	}

	dataplane, err := h.services.DataPlaneService.GetDataPlane(ctx, orgName, dpName)
	if err != nil {
		if errors.Is(err, services.ErrDataPlaneNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "DataPlane not found", services.CodeDataPlaneNotFound)
			return
		}
		h.logger.Error("Failed to get dataplane", "error", err, "org", orgName, "dataplane", dpName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get dataplane", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, dataplane)
}

// CreateDataPlane handles POST /api/v1/orgs/{orgName}/dataplanes
func (h *Handler) CreateDataPlane(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName := r.PathValue("orgName")

	if orgName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
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

	dataplane, err := h.services.DataPlaneService.CreateDataPlane(ctx, orgName, &req)
	if err != nil {
		if errors.Is(err, services.ErrDataPlaneAlreadyExists) {
			writeErrorResponse(w, http.StatusConflict, "DataPlane already exists", services.CodeDataPlaneExists)
			return
		}
		h.logger.Error("Failed to create dataplane", "error", err, "org", orgName, "dataplane", req.Name)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create dataplane", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusCreated, dataplane)
}
