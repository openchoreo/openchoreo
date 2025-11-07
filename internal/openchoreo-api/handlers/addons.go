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

func (h *Handler) ListAddons(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListAddons handler called")

	// Extract organization name from URL path
	orgName := r.PathValue("orgName")
	if orgName == "" {
		logger.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	// Call service to list Addons
	addons, err := h.services.AddonService.ListAddons(ctx, orgName)
	if err != nil {
		logger.Error("Failed to list Addons", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Convert to slice of values for the list response
	addonValues := make([]*models.AddonResponse, len(addons))
	copy(addonValues, addons)

	// Success response with pagination info (simplified for now)
	logger.Debug("Listed Addons successfully", "org", orgName, "count", len(addons))
	writeListResponse(w, addonValues, len(addons), 1, len(addons))
}

func (h *Handler) GetAddonSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetAddonSchema handler called")

	// Extract path parameters
	orgName := r.PathValue("orgName")
	addonName := r.PathValue("addonName")
	if orgName == "" || addonName == "" {
		logger.Warn("Organization name and Addon name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name and Addon name are required", services.CodeInvalidInput)
		return
	}

	// Call service to get Addon schema
	schema, err := h.services.AddonService.GetAddonSchema(ctx, orgName, addonName)
	if err != nil {
		if errors.Is(err, services.ErrAddonNotFound) {
			logger.Warn("Addon not found", "org", orgName, "name", addonName)
			writeErrorResponse(w, http.StatusNotFound, "Addon not found", services.CodeAddonNotFound)
			return
		}
		logger.Error("Failed to get Addon schema", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved Addon schema successfully", "org", orgName, "name", addonName)
	writeSuccessResponse(w, http.StatusOK, schema)
}
