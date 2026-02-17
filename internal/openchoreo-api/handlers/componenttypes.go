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

func (h *Handler) ListComponentTypes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListComponentTypes handler called")

	// Extract namespace name from URL path
	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		logger.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	// Call service to list ComponentTypes
	cts, err := h.services.ComponentTypeService.ListComponentTypes(ctx, namespaceName)
	if err != nil {
		logger.Error("Failed to list ComponentTypes", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Convert to slice of values for the list response
	ctValues := make([]*models.ComponentTypeResponse, len(cts))
	copy(ctValues, cts)

	// Success response with pagination info (simplified for now)
	logger.Debug("Listed ComponentTypes successfully", "namespace", namespaceName, "count", len(cts))
	writeListResponse(w, ctValues, len(cts), 1, len(cts))
}

func (h *Handler) GetComponentTypeSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetComponentTypeSchema handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	ctName := r.PathValue("ctName")
	if namespaceName == "" || ctName == "" {
		logger.Warn("Namespace name and ComponentType name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name and ComponentType name are required", services.CodeInvalidInput)
		return
	}

	// Call service to get ComponentType schema
	schema, err := h.services.ComponentTypeService.GetComponentTypeSchema(ctx, namespaceName, ctName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to view component type schema", "namespace", namespaceName, "componentType", ctName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrComponentTypeNotFound) {
			logger.Warn("ComponentType not found", "namespace", namespaceName, "name", ctName)
			writeErrorResponse(w, http.StatusNotFound, "ComponentType not found", services.CodeComponentTypeNotFound)
			return
		}
		logger.Error("Failed to get ComponentType schema", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved ComponentType schema successfully", "namespace", namespaceName, "name", ctName)
	writeSuccessResponse(w, http.StatusOK, schema)
}
