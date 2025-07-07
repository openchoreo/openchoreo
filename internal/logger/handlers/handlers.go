// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/logger/httputil"
	"github.com/openchoreo/openchoreo/internal/logger/opensearch"
	"github.com/openchoreo/openchoreo/internal/logger/service"
)

const (
	defaultSortOrder = "desc"
)

// Error codes and messages
const (
	// Error types
	ErrorTypeMissingParameter = "missingParameter"
	ErrorTypeInvalidRequest   = "invalidRequest"
	ErrorTypeInternalError    = "internalError"

	// Error codes
	ErrorCodeMissingParameter = "OBS-L-10"
	ErrorCodeInvalidRequest   = "OBS-L-12"
	ErrorCodeInternalError    = "OBS-L-25"

	// Error messages
	ErrorMsgComponentIDRequired    = "Component ID is required"
	ErrorMsgProjectIDRequired      = "Project ID is required"
	ErrorMsgOrganizationIDRequired = "Organization ID is required"
	ErrorMsgInvalidRequestFormat   = "Invalid request format"
	ErrorMsgFailedToRetrieveLogs   = "Failed to retrieve logs"
)

// Handler contains the HTTP handlers for the logging API
type Handler struct {
	service *service.LoggingService
	logger  *slog.Logger
}

// NewHandler creates a new handler instance
func NewHandler(service *service.LoggingService, logger *slog.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// writeJSON writes JSON response and logs any error
func (h *Handler) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	if err := httputil.WriteJSON(w, status, v); err != nil {
		h.logger.Error("Failed to write JSON response", "error", err)
	}
}

// writeErrorResponse writes a standardized error response
func (h *Handler) writeErrorResponse(w http.ResponseWriter, status int, errorType, code, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error:   errorType,
		Code:    code,
		Message: message,
	})
}

// ComponentLogsRequest represents the request body for component logs
type ComponentLogsRequest struct {
	StartTime     string   `json:"startTime" validate:"required"`
	EndTime       string   `json:"endTime" validate:"required"`
	EnvironmentID string   `json:"environmentId" validate:"required"`
	Namespace     string   `json:"namespace" validate:"required"`
	SearchPhrase  string   `json:"searchPhrase,omitempty"`
	LogLevels     []string `json:"logLevels,omitempty"`
	Versions      []string `json:"versions,omitempty"`
	VersionIDs    []string `json:"versionIds,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	SortOrder     string   `json:"sortOrder,omitempty"`
}

// ProjectLogsRequest represents the request body for project logs
type ProjectLogsRequest struct {
	ComponentLogsRequest
	ComponentIDs []string `json:"componentIds,omitempty"`
}

// GatewayLogsRequest represents the request body for gateway logs
type GatewayLogsRequest struct {
	StartTime         string            `json:"startTime" validate:"required"`
	EndTime           string            `json:"endTime" validate:"required"`
	OrganizationID    string            `json:"organizationId" validate:"required"`
	SearchPhrase      string            `json:"searchPhrase,omitempty"`
	APIIDToVersionMap map[string]string `json:"apiIdToVersionMap,omitempty"`
	GatewayVHosts     []string          `json:"gatewayVHosts,omitempty"`
	Limit             int               `json:"limit,omitempty"`
	SortOrder         string            `json:"sortOrder,omitempty"`
}

// OrganizationLogsRequest represents the request body for organization logs
type OrganizationLogsRequest struct {
	ComponentLogsRequest
	PodLabels map[string]string `json:"podLabels,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// GetComponentLogs handles POST /api/logs/component/{componentId}
func (h *Handler) GetComponentLogs(w http.ResponseWriter, r *http.Request) {
	componentID := httputil.GetPathParam(r, "componentId")
	if componentID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgComponentIDRequired)
		return
	}

	var req ComponentLogsRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidRequestFormat)
		return
	}

	// Set defaults
	if req.Limit == 0 {
		req.Limit = 100
	}
	if req.SortOrder == "" {
		req.SortOrder = defaultSortOrder
	}

	// Build query parameters
	params := opensearch.QueryParams{
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		SearchPhrase:  req.SearchPhrase,
		LogLevels:     req.LogLevels,
		Limit:         req.Limit,
		SortOrder:     req.SortOrder,
		ComponentID:   componentID,
		EnvironmentID: req.EnvironmentID,
		Namespace:     req.Namespace,
		Versions:      req.Versions,
		VersionIDs:    req.VersionIDs,
	}

	// Execute query
	ctx := r.Context()
	result, err := h.service.GetComponentLogs(ctx, params)
	if err != nil {
		h.logger.Error("Failed to get component logs", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToRetrieveLogs)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// GetProjectLogs handles POST /api/logs/project/{projectId}
func (h *Handler) GetProjectLogs(w http.ResponseWriter, r *http.Request) {
	projectID := httputil.GetPathParam(r, "projectId")
	if projectID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgProjectIDRequired)
		return
	}

	var req ProjectLogsRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidRequestFormat)
		return
	}

	// Set defaults
	if req.Limit == 0 {
		req.Limit = 100
	}
	if req.SortOrder == "" {
		req.SortOrder = defaultSortOrder
	}

	// Build query parameters
	params := opensearch.QueryParams{
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		SearchPhrase:  req.SearchPhrase,
		LogLevels:     req.LogLevels,
		Limit:         req.Limit,
		SortOrder:     req.SortOrder,
		ProjectID:     projectID,
		EnvironmentID: req.EnvironmentID,
		Versions:      req.Versions,
		VersionIDs:    req.VersionIDs,
	}

	// Execute query
	ctx := r.Context()
	result, err := h.service.GetProjectLogs(ctx, params, req.ComponentIDs)
	if err != nil {
		h.logger.Error("Failed to get project logs", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToRetrieveLogs)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// GetGatewayLogs handles POST /api/logs/gateway
func (h *Handler) GetGatewayLogs(w http.ResponseWriter, r *http.Request) {
	var req GatewayLogsRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidRequestFormat)
		return
	}

	// Set defaults
	if req.Limit == 0 {
		req.Limit = 100
	}
	if req.SortOrder == "" {
		req.SortOrder = defaultSortOrder
	}

	// Build query parameters
	params := opensearch.GatewayQueryParams{
		QueryParams: opensearch.QueryParams{
			StartTime:    req.StartTime,
			EndTime:      req.EndTime,
			SearchPhrase: req.SearchPhrase,
			Limit:        req.Limit,
			SortOrder:    req.SortOrder,
		},
		OrganizationID:    req.OrganizationID,
		APIIDToVersionMap: req.APIIDToVersionMap,
		GatewayVHosts:     req.GatewayVHosts,
	}

	// Execute query
	ctx := r.Context()
	result, err := h.service.GetGatewayLogs(ctx, params)
	if err != nil {
		h.logger.Error("Failed to get gateway logs", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToRetrieveLogs)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// GetOrganizationLogs handles POST /api/logs/org/{orgId}
func (h *Handler) GetOrganizationLogs(w http.ResponseWriter, r *http.Request) {
	orgID := httputil.GetPathParam(r, "orgId")
	if orgID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgOrganizationIDRequired)
		return
	}

	var req OrganizationLogsRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidRequestFormat)
		return
	}

	// Set defaults
	if req.Limit == 0 {
		req.Limit = 100
	}
	if req.SortOrder == "" {
		req.SortOrder = defaultSortOrder
	}

	// Build query parameters
	params := opensearch.QueryParams{
		StartTime:      req.StartTime,
		EndTime:        req.EndTime,
		SearchPhrase:   req.SearchPhrase,
		LogLevels:      req.LogLevels,
		Limit:          req.Limit,
		SortOrder:      req.SortOrder,
		EnvironmentID:  req.EnvironmentID,
		Namespace:      req.Namespace,
		Versions:       req.Versions,
		VersionIDs:     req.VersionIDs,
		OrganizationID: orgID, // Add the organization ID from URL parameter
	}

	// Execute query
	ctx := r.Context()
	result, err := h.service.GetOrganizationLogs(ctx, params, req.PodLabels)
	if err != nil {
		h.logger.Error("Failed to get organization logs", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToRetrieveLogs)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// Health handles GET /health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.service.HealthCheck(ctx); err != nil {
		h.writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
