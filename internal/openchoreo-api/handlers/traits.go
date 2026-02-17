// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
)

func (h *Handler) ListTraits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListTraits handler called")

	// Extract namespace name from URL path
	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		logger.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	// Call service to list Traits
	traits, err := h.services.TraitService.ListTraits(ctx, namespaceName)
	if err != nil {
		logger.Error("Failed to list Traits", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Convert to slice of values for the list response
	traitValues := make([]*models.TraitResponse, len(traits))
	copy(traitValues, traits)

	// Success response with pagination info (simplified for now)
	logger.Debug("Listed Traits successfully", "namespace", namespaceName, "count", len(traits))
	writeListResponse(w, traitValues, len(traits), 1, len(traits))
}

func (h *Handler) GetTraitSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetTraitSchema handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	traitName := r.PathValue("traitName")
	if namespaceName == "" || traitName == "" {
		logger.Warn("Namespace name and Trait name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name and Trait name are required", services.CodeInvalidInput)
		return
	}

	// Call service to get Trait schema
	schema, err := h.services.TraitService.GetTraitSchema(ctx, namespaceName, traitName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to view trait schema", "namespace", namespaceName, "trait", traitName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrTraitNotFound) {
			logger.Warn("Trait not found", "namespace", namespaceName, "name", traitName)
			writeErrorResponse(w, http.StatusNotFound, "Trait not found", services.CodeTraitNotFound)
			return
		}
		logger.Error("Failed to get Trait schema", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved Trait schema successfully", "namespace", namespaceName, "name", traitName)
	writeSuccessResponse(w, http.StatusOK, schema)
}
