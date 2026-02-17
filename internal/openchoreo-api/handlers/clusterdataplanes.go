// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

const defaultLimit = 20
const maxLimit = 100

// ListClusterDataPlanes handles GET /api/v1/clusterdataplanes
func (h *Handler) ListClusterDataPlanes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limit := defaultLimit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed < 1 || parsed > maxLimit {
			writeErrorResponse(w, http.StatusBadRequest,
				"limit must be an integer between 1 and 100", services.CodeInvalidInput)
			return
		}
		limit = parsed
	}
	cursor := r.URL.Query().Get("cursor")

	result, err := h.services.ClusterDataPlaneService.ListClusterDataPlanesPaginated(ctx, limit, cursor)
	if err != nil {
		h.logger.Error("Failed to list cluster dataplanes", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list cluster dataplanes", services.CodeInternalError)
		return
	}

	writeCursorListResponse(w, result.Items, result.NextCursor, result.RemainingCount)
}

// GetClusterDataPlane handles GET /api/v1/clusterdataplanes/{cdpName}
func (h *Handler) GetClusterDataPlane(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cdpName := r.PathValue("cdpName")

	if cdpName == "" {
		h.logger.Error("Missing required path parameter", "param", "cdpName")
		writeErrorResponse(w, http.StatusBadRequest, "ClusterDataPlane name is required", services.CodeInvalidInput)
		return
	}

	dataplane, err := h.services.ClusterDataPlaneService.GetClusterDataPlane(ctx, cdpName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to view cluster dataplane", "clusterDataPlane", cdpName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrClusterDataPlaneNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "ClusterDataPlane not found", services.CodeClusterDataPlaneNotFound)
			return
		}
		h.logger.Error("Failed to get cluster dataplane", "error", err, "clusterDataPlane", cdpName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get cluster dataplane", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, dataplane)
}

// CreateClusterDataPlane handles POST /api/v1/clusterdataplanes
func (h *Handler) CreateClusterDataPlane(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req models.CreateClusterDataPlaneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to decode request body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		h.logger.Error("Request validation failed", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	setAuditResource(ctx, "clusterdataplane", req.Name, req.Name)

	dataplane, err := h.services.ClusterDataPlaneService.CreateClusterDataPlane(ctx, &req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to create cluster dataplane", "clusterDataPlane", req.Name)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrClusterDataPlaneAlreadyExists) {
			writeErrorResponse(w, http.StatusConflict, "ClusterDataPlane already exists", services.CodeClusterDataPlaneExists)
			return
		}
		h.logger.Error("Failed to create cluster dataplane", "error", err, "clusterDataPlane", req.Name)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create cluster dataplane", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusCreated, dataplane)
}
