// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Info("CreateProject handler called")

	// Extract organization name from URL path
	orgName := r.PathValue("orgName")
	if orgName == "" {
		logger.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest,
			"Organization name is required", "INVALID_ORG_NAME")
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

	// Call service to create project
	project, err := h.services.ProjectService.CreateProject(ctx, orgName, &req)
	if err != nil {
		if errors.Is(err, services.ErrProjectAlreadyExists) {
			logger.Warn("Project already exists", "org", orgName, "project", req.Name)
			writeErrorResponse(w, http.StatusConflict, "Project already exists", services.CodeProjectExists)
			return
		}
		logger.Error("Failed to create project", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Info("Project created successfully", "org", orgName, "project", project.Name)
	writeSuccessResponse(w, http.StatusCreated, project)
}

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListProjects handler called")

	// Extract organization name from URL path
	orgName := r.PathValue("orgName")
	if orgName == "" {
		logger.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest,
			"Organization name is required", services.CodeInvalidInput)
		return
	}

	// Check for cursor-based pagination
	cursor, limit, useCursor, err := parseCursorParams(r)
	if err != nil {
		errorCode := services.GetPaginationErrorCode(err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), errorCode)
		return
	}

	if useCursor {
		// only validate non-empty cursors
		if cursor != "" {
			if err := validateCursorWithContext(cursor); err != nil {
				logger.Warn("Invalid cursor", "error", err, "ip", r.RemoteAddr)
				writeErrorResponse(w, http.StatusBadRequest,
					"Invalid cursor parameter", services.CodeInvalidCursorFormat)
				return
			}
		}

		projects, nextCursor, err := h.services.ProjectService.ListProjectsWithCursor(
			ctx, orgName, cursor, limit)
		if err != nil {
			if errors.Is(err, services.ErrOrganizationNotFound) {
				writeErrorResponse(w, http.StatusNotFound,
					"Organization not found", services.CodeOrganizationNotFound)
				return
			}
			if errors.Is(err, services.ErrContinueTokenExpired) {
				writeTokenExpiredError(w)
				return
			}
			if errors.Is(err, services.ErrInvalidCursorFormat) {
				logger.Error("Invalid cursor format", "error", err, "ip", r.RemoteAddr)
				writeErrorResponse(w, http.StatusBadRequest,
					"Invalid cursor format", services.CodeInvalidCursorFormat)
				return
			}
			if strings.Contains(err.Error(), "service unavailable") {
				writeErrorResponse(w, http.StatusServiceUnavailable,
					"Service temporarily unavailable", services.CodeInternalError)
				return
			}
			logger.Error("Failed to list projects with cursor", "error", err)
			writeErrorResponse(w, http.StatusInternalServerError,
				"Internal server error", services.CodeInternalError)
			return
		}

		writeCursorListResponse(w, projects, nextCursor)
		return
	}

	// Legacy mode
	// Call service to list projects
	projects, err := h.services.ProjectService.ListProjects(ctx, orgName)
	if err != nil {
		logger.Error("Failed to list projects", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError,
			"Internal server error", services.CodeInternalError)
		return
	}

	// Convert to slice of values for the list response
	projectValues := make([]*models.ProjectResponse, len(projects))
	copy(projectValues, projects)

	// Success response with pagination info (simplified for now)
	logger.Debug("Listed projects successfully", "org", orgName, "count", len(projects))
	writeListResponse(w, projectValues, len(projects), 1, len(projects))
}

func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetProject handler called")

	// Extract path parameters
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	if orgName == "" || projectName == "" {
		logger.Warn("Organization name and project name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name and project name are required", "INVALID_PARAMS")
		return
	}

	// Call service to get project
	project, err := h.services.ProjectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "org", orgName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		logger.Error("Failed to get project", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved project successfully", "org", orgName, "project", projectName)
	writeSuccessResponse(w, http.StatusOK, project)
}
