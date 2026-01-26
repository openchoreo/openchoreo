// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
)

func (h *Handler) ListTraits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListTraits handler called")

	// Extract organization name from URL path
	orgName := r.PathValue("orgName")
	if orgName == "" {
		logger.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	// Extract and validate list parameters
	opts, err := extractListParams(r.URL.Query())
	if err != nil {
		logger.Warn("Invalid list parameters", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	// Call service to list Traits
	result, err := h.services.TraitService.ListTraits(ctx, orgName, opts)
	if err != nil {
		if handlePaginationError(w, err, logger) {
			return
		}
		logger.Error("Failed to list Traits", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Listed Traits successfully", "org", orgName, "count", len(result.Items), "hasMore", result.Metadata.HasMore)
	writeListResponse(w, result.Items, result.Metadata.ResourceVersion, result.Metadata.Continue)
}

func (h *Handler) GetTraitSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetTraitSchema handler called")

	// Extract path parameters
	orgName := r.PathValue("orgName")
	traitName := r.PathValue("traitName")
	if orgName == "" || traitName == "" {
		logger.Warn("Organization name and Trait name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name and Trait name are required", services.CodeInvalidInput)
		return
	}

	// Call service to get Trait schema
	schema, err := h.services.TraitService.GetTraitSchema(ctx, orgName, traitName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to view trait schema", "org", orgName, "trait", traitName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrTraitNotFound) {
			logger.Warn("Trait not found", "org", orgName, "name", traitName)
			writeErrorResponse(w, http.StatusNotFound, "Trait not found", services.CodeTraitNotFound)
			return
		}
		logger.Error("Failed to get Trait schema", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved Trait schema successfully", "org", orgName, "name", traitName)
	writeSuccessResponse(w, http.StatusOK, schema)
}
