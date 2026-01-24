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

// ListEnvironments handles GET /api/v1/orgs/{orgName}/environments
func (h *Handler) ListEnvironments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName := r.PathValue("orgName")

	if orgName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	// Extract and validate list parameters
	opts, err := extractListParams(r.URL.Query())
	if err != nil {
		h.logger.Warn("Invalid list parameters", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	result, err := h.services.EnvironmentService.ListEnvironments(ctx, orgName, opts)
	if err != nil {
		if errors.Is(err, services.ErrContinueTokenExpired) {
			h.logger.Warn("Continue token expired")
			writeErrorResponse(w, http.StatusGone,
				"Continue token has expired. Please restart listing from the beginning.",
				services.CodeContinueTokenExpired)
			return
		}
		if errors.Is(err, services.ErrInvalidContinueToken) {
			h.logger.Warn("Invalid continue token provided")
			writeErrorResponse(w, http.StatusBadRequest,
				"Invalid continue token provided",
				services.CodeInvalidContinueToken)
			return
		}
		h.logger.Error("Failed to list environments", "error", err, "org", orgName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list environments", services.CodeInternalError)
		return
	}

	h.logger.Debug("Listed environments successfully", "count", len(result.Items), "org", orgName, "hasMore", result.Metadata.HasMore)
	writeListResponse(w, result.Items, result.Metadata.ResourceVersion, result.Metadata.Continue)
}

// GetEnvironment handles GET /api/v1/orgs/{orgName}/environments/{envName}
func (h *Handler) GetEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName := r.PathValue("orgName")
	envName := r.PathValue("envName")

	if orgName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	if envName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Environment name is required", services.CodeInvalidInput)
		return
	}

	environment, err := h.services.EnvironmentService.GetEnvironment(ctx, orgName, envName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to view environment", "org", orgName, "env", envName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrEnvironmentNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Environment not found", services.CodeEnvironmentNotFound)
			return
		}
		h.logger.Error("Failed to get environment", "error", err, "org", orgName, "env", envName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get environment", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, environment)
}

// CreateEnvironment handles POST /api/v1/orgs/{orgName}/environments
func (h *Handler) CreateEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName := r.PathValue("orgName")

	if orgName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	var req models.CreateEnvironmentRequest
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

	setAuditResource(ctx, "environment", req.Name, req.Name)
	addAuditMetadata(ctx, "organization", orgName)

	environment, err := h.services.EnvironmentService.CreateEnvironment(ctx, orgName, &req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to create environment", "org", orgName, "env", req.Name)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrEnvironmentAlreadyExists) {
			writeErrorResponse(w, http.StatusConflict, "Environment already exists", services.CodeEnvironmentExists)
			return
		}
		h.logger.Error("Failed to create environment", "error", err, "org", orgName, "env", req.Name)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create environment", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusCreated, environment)
}

// GetEnvironmentObserverURL handles GET /api/v1/orgs/{orgName}/environments/{envName}/observer-url
func (h *Handler) GetEnvironmentObserverURL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName := r.PathValue("orgName")
	envName := r.PathValue("envName")

	if orgName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	if envName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Environment name is required", services.CodeInvalidInput)
		return
	}

	observerResponse, err := h.services.EnvironmentService.GetEnvironmentObserverURL(ctx, orgName, envName)
	if err != nil {
		if errors.Is(err, services.ErrEnvironmentNotFound) {
			h.logger.Warn("Environment not found", "org", orgName, "env", envName)
			writeErrorResponse(w, http.StatusNotFound, "Environment not found", services.CodeEnvironmentNotFound)
			return
		}
		if errors.Is(err, services.ErrDataPlaneNotFound) {
			h.logger.Warn("DataPlane not found", "org", orgName, "env", envName)
			writeErrorResponse(w, http.StatusNotFound, "DataPlane not found", services.CodeDataPlaneNotFound)
			return
		}
		h.logger.Error("Failed to get environment observer URL", "error", err, "org", orgName, "env", envName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get environment observer URL", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, observerResponse)
}
