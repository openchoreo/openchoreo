// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

func (h *Handler) ListComponentTypes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListComponentTypes handler called")

	// Extract organization name from URL path
	orgName := r.PathValue("orgName")
	if orgName == "" {
		logger.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	// Call service to list ComponentTypes
	ctds, err := h.services.ComponentTypeService.ListComponentTypes(ctx, orgName)
	if err != nil {
		logger.Error("Failed to list ComponentTypes", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Convert to slice of values for the list response
	ctdValues := make([]*models.ComponentTypeResponse, len(ctds))
	copy(ctdValues, ctds)

	// Success response with pagination info (simplified for now)
	logger.Debug("Listed ComponentTypes successfully", "org", orgName, "count", len(ctds))
	writeListResponse(w, ctdValues, len(ctds), 1, len(ctds))
}

func (h *Handler) GetComponentTypeSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetComponentTypeSchema handler called")

	// Extract path parameters
	orgName := r.PathValue("orgName")
	ctdName := r.PathValue("ctdName")
	if orgName == "" || ctdName == "" {
		logger.Warn("Organization name and ComponentType name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name and ComponentType name are required", services.CodeInvalidInput)
		return
	}

	// Call service to get ComponentType schema
	schema, err := h.services.ComponentTypeService.GetComponentTypeSchema(ctx, orgName, ctdName)
	if err != nil {
		if errors.Is(err, services.ErrComponentTypeNotFound) {
			logger.Warn("ComponentType not found", "org", orgName, "name", ctdName)
			writeErrorResponse(w, http.StatusNotFound, "ComponentType not found", services.CodeComponentTypeNotFound)
			return
		}
		logger.Error("Failed to get ComponentType schema", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved ComponentType schema successfully", "org", orgName, "name", ctdName)
	writeSuccessResponse(w, http.StatusOK, schema)
}
