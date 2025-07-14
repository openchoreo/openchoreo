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

	environments, err := h.services.EnvironmentService.ListEnvironments(ctx, orgName)
	if err != nil {
		h.logger.Error("Failed to list environments", "error", err, "org", orgName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list environments", services.CodeInternalError)
		return
	}

	writeListResponse(w, environments, len(environments), 1, len(environments))
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

	environment, err := h.services.EnvironmentService.CreateEnvironment(ctx, orgName, &req)
	if err != nil {
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