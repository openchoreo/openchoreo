// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
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

	// Extract and validate list parameters
	opts, err := extractListParams(r.URL.Query())
	if err != nil {
		logger.Warn("Invalid list parameters", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	// Call service to list ComponentTypes
	result, err := h.services.ComponentTypeService.ListComponentTypes(ctx, orgName, opts)
	if err != nil {
		if handlePaginationError(w, err, logger) {
			return
		}
		logger.Error("Failed to list ComponentTypes", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Listed ComponentTypes successfully", "org", orgName, "count", len(result.Items), "hasMore", result.Metadata.HasMore)
	writeListResponse(w, result.Items, result.Metadata.ResourceVersion, result.Metadata.Continue)
}

func (h *Handler) GetComponentTypeSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetComponentTypeSchema handler called")

	// Extract path parameters
	orgName := r.PathValue("orgName")
	ctName := r.PathValue("ctName")
	if orgName == "" || ctName == "" {
		logger.Warn("Organization name and ComponentType name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name and ComponentType name are required", services.CodeInvalidInput)
		return
	}

	// Call service to get ComponentType schema
	schema, err := h.services.ComponentTypeService.GetComponentTypeSchema(ctx, orgName, ctName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to view component type schema", "org", orgName, "componentType", ctName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrComponentTypeNotFound) {
			logger.Warn("ComponentType not found", "org", orgName, "name", ctName)
			writeErrorResponse(w, http.StatusNotFound, "ComponentType not found", services.CodeComponentTypeNotFound)
			return
		}
		logger.Error("Failed to get ComponentType schema", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved ComponentType schema successfully", "org", orgName, "name", ctName)
	writeSuccessResponse(w, http.StatusOK, schema)
}
