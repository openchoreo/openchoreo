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

func (h *Handler) ListComponentTypeDefinitions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListComponentTypeDefinitions handler called")

	// Extract organization name from URL path
	orgName := r.PathValue("orgName")
	if orgName == "" {
		logger.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	// Call service to list ComponentTypeDefinitions
	ctds, err := h.services.ComponentTypeDefinitionService.ListComponentTypeDefinitions(ctx, orgName)
	if err != nil {
		logger.Error("Failed to list ComponentTypeDefinitions", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Convert to slice of values for the list response
	ctdValues := make([]*models.ComponentTypeDefinitionResponse, len(ctds))
	copy(ctdValues, ctds)

	// Success response with pagination info (simplified for now)
	logger.Debug("Listed ComponentTypeDefinitions successfully", "org", orgName, "count", len(ctds))
	writeListResponse(w, ctdValues, len(ctds), 1, len(ctds))
}

func (h *Handler) GetComponentTypeDefinitionSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetComponentTypeDefinitionSchema handler called")

	// Extract path parameters
	orgName := r.PathValue("orgName")
	ctdName := r.PathValue("ctdName")
	if orgName == "" || ctdName == "" {
		logger.Warn("Organization name and ComponentTypeDefinition name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name and ComponentTypeDefinition name are required", services.CodeInvalidInput)
		return
	}

	// Call service to get ComponentTypeDefinition schema
	schema, err := h.services.ComponentTypeDefinitionService.GetComponentTypeDefinitionSchema(ctx, orgName, ctdName)
	if err != nil {
		if errors.Is(err, services.ErrComponentTypeDefinitionNotFound) {
			logger.Warn("ComponentTypeDefinition not found", "org", orgName, "name", ctdName)
			writeErrorResponse(w, http.StatusNotFound, "ComponentTypeDefinition not found", services.CodeComponentTypeDefinitionNotFound)
			return
		}
		logger.Error("Failed to get ComponentTypeDefinition schema", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved ComponentTypeDefinition schema successfully", "org", orgName, "name", ctdName)
	writeSuccessResponse(w, http.StatusOK, schema)
}
