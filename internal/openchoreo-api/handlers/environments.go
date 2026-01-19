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

// ListEnvironments handles GET /api/v1/namespaces/{namespaceName}/environments
func (h *Handler) ListEnvironments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	namespaceName := r.PathValue("namespaceName")

	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	environments, err := h.services.EnvironmentService.ListEnvironments(ctx, namespaceName)
	if err != nil {
		h.logger.Error("Failed to list environments", "error", err, "org", namespaceName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list environments", services.CodeInternalError)
		return
	}

	writeListResponse(w, environments, len(environments), 1, len(environments))
}

// GetEnvironment handles GET /api/v1/namespaces/{namespaceName}/environments/{envName}
func (h *Handler) GetEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	namespaceName := r.PathValue("namespaceName")
	envName := r.PathValue("envName")

	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	if envName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Environment name is required", services.CodeInvalidInput)
		return
	}

	environment, err := h.services.EnvironmentService.GetEnvironment(ctx, namespaceName, envName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to view environment", "org", namespaceName, "env", envName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrEnvironmentNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Environment not found", services.CodeEnvironmentNotFound)
			return
		}
		h.logger.Error("Failed to get environment", "error", err, "org", namespaceName, "env", envName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get environment", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, environment)
}

// CreateEnvironment handles POST /api/v1/namespaces/{namespaceName}/environments
func (h *Handler) CreateEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	namespaceName := r.PathValue("namespaceName")

	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
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
	addAuditMetadata(ctx, "organization", namespaceName)

	environment, err := h.services.EnvironmentService.CreateEnvironment(ctx, namespaceName, &req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to create environment", "org", namespaceName, "env", req.Name)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrEnvironmentAlreadyExists) {
			writeErrorResponse(w, http.StatusConflict, "Environment already exists", services.CodeEnvironmentExists)
			return
		}
		h.logger.Error("Failed to create environment", "error", err, "org", namespaceName, "env", req.Name)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create environment", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusCreated, environment)
}

// GetEnvironmentObserverURL handles GET /api/v1/namespaces/{namespaceName}/environments/{envName}/observer-url
func (h *Handler) GetEnvironmentObserverURL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	namespaceName := r.PathValue("namespaceName")
	envName := r.PathValue("envName")

	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	if envName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Environment name is required", services.CodeInvalidInput)
		return
	}

	observerResponse, err := h.services.EnvironmentService.GetEnvironmentObserverURL(ctx, namespaceName, envName)
	if err != nil {
		if errors.Is(err, services.ErrEnvironmentNotFound) {
			h.logger.Warn("Environment not found", "org", namespaceName, "env", envName)
			writeErrorResponse(w, http.StatusNotFound, "Environment not found", services.CodeEnvironmentNotFound)
			return
		}
		if errors.Is(err, services.ErrDataPlaneNotFound) {
			h.logger.Warn("DataPlane not found", "org", namespaceName, "env", envName)
			writeErrorResponse(w, http.StatusNotFound, "DataPlane not found", services.CodeDataPlaneNotFound)
			return
		}
		h.logger.Error("Failed to get environment observer URL", "error", err, "org", namespaceName, "env", envName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get environment observer URL", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, observerResponse)
}
