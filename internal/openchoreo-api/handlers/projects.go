// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacy_services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
)

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Info("CreateProject handler called")

	// Extract namespace name from URL path
	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		logger.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", "INVALID_NAMESPACE_NAME")
		return
	}

	// Parse request body
	var req models.CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid JSON body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", "INVALID_JSON")
		return
	}
	defer r.Body.Close()

	// Validate request
	if err := req.Validate(); err != nil {
		logger.Error("Validation failed", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), "INVALID_REQUEST")
		return
	}

	// Call service to create project
	project, err := h.services.ProjectService.CreateProject(ctx, namespaceName, &req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to create project", "namespace", namespaceName, "project", req.Name)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectAlreadyExists) {
			logger.Warn("Project already exists", "namespace", namespaceName, "project", req.Name)
			writeErrorResponse(w, http.StatusConflict, "Project already exists", services.CodeProjectExists)
			return
		}
		logger.Error("Failed to create project", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Set audit context for successful creation
	setAuditResource(ctx, "project", project.Name, project.Name)
	addAuditMetadata(ctx, "namespace", namespaceName)

	// Success response
	logger.Info("Project created successfully", "namespace", namespaceName, "project", project.Name)
	writeSuccessResponse(w, http.StatusCreated, project)
}

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListProjects handler called")

	// Extract namespace name from URL path
	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		logger.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", "INVALID_NAMESPACE_NAME")
		return
	}

	// Call service to list projects
	projects, err := h.services.ProjectService.ListProjects(ctx, namespaceName)
	if err != nil {
		logger.Error("Failed to list projects", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Convert to slice of values for the list response
	projectValues := make([]*models.ProjectResponse, len(projects))
	copy(projectValues, projects)

	// Success response with pagination info (simplified for now)
	logger.Debug("Listed projects successfully", "namespace", namespaceName, "count", len(projects))
	writeListResponse(w, projectValues, len(projects), 1, len(projects))
}

func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetProject handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	if namespaceName == "" || projectName == "" {
		logger.Warn("Namespace name and project name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name and project name are required", "INVALID_PARAMS")
		return
	}

	// Call service to get project
	project, err := h.services.ProjectService.GetProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to view project", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		logger.Error("Failed to get project", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved project successfully", "namespace", namespaceName, "project", projectName)
	writeSuccessResponse(w, http.StatusOK, project)
}

func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Info("DeleteProject handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	if namespaceName == "" || projectName == "" {
		logger.Warn("Namespace name and project name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name and project name are required", "INVALID_PARAMS")
		return
	}

	// Call service to delete project
	err := h.services.ProjectService.DeleteProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to delete project", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		logger.Error("Failed to delete project", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response - 204 No Content for successful delete
	logger.Info("Project deleted successfully", "namespace", namespaceName, "project", projectName)
	w.WriteHeader(http.StatusNoContent)
}
