// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

const (
	defaultSortOrder   = "desc"
	ascendingSortOrder = "asc"
)

const (
	defaultWorkflowRunLogsLimit = 1000
)

// isAIRCAEnabled checks if AI RCA analysis is enabled via environment variable
func isAIRCAEnabled() bool {
	enabled, _ := strconv.ParseBool(os.Getenv("AI_RCA_ENABLED"))
	return enabled
}

// RequireRCA wraps a handler and returns 503 if AI RCA is not enabled
func RequireRCA(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAIRCAEnabled() {
			http.Error(w, "RCA service is not enabled", http.StatusServiceUnavailable)
			return
		}
		next(w, r)
	}
}

// Error codes and messages
const (
	// Error types
	ErrorTypeMissingParameter   = "missingParameter"
	ErrorTypeInvalidRequest     = "invalidRequest"
	ErrorTypeInternalError      = "internalError"
	ErrorTypeForbidden          = "forbidden"
	ErrorTypeUnauthorized       = "unauthorized"
	ErrorTypeServiceUnavailable = "serviceUnavailable"

	// Error codes
	ErrorCodeMissingParameter = "OBS-L-10"
	ErrorCodeInvalidRequest   = "OBS-L-12"
	ErrorCodeInternalError    = "OBS-L-25"
	ErrorCodeAuthForbidden    = "OBS-AUTH-01"
	ErrorCodeAuthUnavailable  = "OBS-AUTH-02"
	ErrorCodeAuthUnauthorized = "OBS-AUTH-04"

	// Error messages
	ErrorMsgBuildIDRequired           = "Build ID is required"
	ErrorMsgWorkflowRunIDRequired     = "Workflow run ID is required"
	ErrorMsgComponentIDRequired       = "Component ID is required"
	ErrorMsgProjectIDRequired         = "Project ID is required"
	ErrorMsgNamespaceNameRequired     = "Namespace name is required"
	ErrorMsgEnvironmentIDRequired     = "Environment ID is required"
	ErrorMsgRuleNameRequired          = "Rule name is required"
	ErrorMsgSourceTypeRequired        = "Source type is required"
	ErrorMsgAlertIDRequired           = "Alert ID is required"
	ErrorMsgTimeRequired              = "Start time and end time are required"
	ErrorMsgInvalidRequestFormat      = "Invalid request format"
	ErrorMsgFailedToRetrieveLogs      = "Failed to retrieve logs"
	ErrorMsgFailedToRetrieveMetrics   = "Failed to retrieve metrics"
	ErrorMsgInvalidTimeFormat         = "Invalid time format"
	ErrorMsgAccessDenied              = "access denied due to insufficient permissions"
	ErrorMsgAuthServiceUnavailable    = "Authorization service temporarily unavailable"
	ErrorMsgFailedToAuthorize         = "Failed to authorize request"
	ErrorMsgUnauthorized              = "Unauthorized request"
	ErrorMsgMissingAuthHierarchy      = "missing required fields for authorization"
	LogMsgAuthServiceUnavailableError = "Authorization service unavailable or timed out"
	ErrorMsgAlertSourceRequired       = "Alert source is required"
)

// Handler contains the HTTP handlers for the logging API
type Handler struct {
	service       *service.LoggingService
	logger        *slog.Logger
	authzPDP      authzcore.PDP
	rcaServiceURL string
}

// NewHandler creates a new handler instance
func NewHandler(service *service.LoggingService, logger *slog.Logger, authzPDP authzcore.PDP, rcaServiceURL string) *Handler {
	return &Handler{
		service:       service,
		logger:        logger,
		authzPDP:      authzPDP,
		rcaServiceURL: rcaServiceURL,
	}
}

// writeJSON writes JSON response and logs any error
func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
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

// BuildLogsRequest represents the request body for build logs
type BuildLogsRequest struct {
	ComponentName string `json:"componentName,omitempty"`
	NamespaceName string `json:"namespaceName,omitempty"`
	ProjectName   string `json:"projectName,omitempty"`
	StartTime     string `json:"startTime" validate:"required"`
	EndTime       string `json:"endTime" validate:"required"`
	Limit         int    `json:"limit,omitempty"`
	SortOrder     string `json:"sortOrder,omitempty"`
}

// WorkflowRunLogsRequest represents the request body for workflow run logs
type WorkflowRunLogsRequest struct {
	NamespaceName string `json:"namespaceName" validate:"required"`
	StartTime     string `json:"startTime" validate:"required"`
	EndTime       string `json:"endTime" validate:"required"`
	Limit         int    `json:"limit,omitempty"`
	SortOrder     string `json:"sortOrder,omitempty"`
}

// ComponentLogsRequest represents the request body for component logs
type ComponentLogsRequest struct {
	ComponentName   string   `json:"componentName,omitempty"`
	NamespaceName   string   `json:"namespaceName,omitempty"`
	StartTime       string   `json:"startTime" validate:"required"`
	EndTime         string   `json:"endTime" validate:"required"`
	EnvironmentName string   `json:"environmentName,omitempty"`
	EnvironmentID   string   `json:"environmentId" validate:"required"`
	Namespace       string   `json:"namespace" validate:"required"`
	ProjectName     string   `json:"projectName,omitempty"`
	SearchPhrase    string   `json:"searchPhrase,omitempty"`
	LogLevels       []string `json:"logLevels,omitempty"`
	Versions        []string `json:"versions,omitempty"`
	VersionIDs      []string `json:"versionIds,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	SortOrder       string   `json:"sortOrder,omitempty"`
	LogType         string   `json:"logType,omitempty"`
	BuildID         string   `json:"buildId,omitempty"`
	BuildUUID       string   `json:"buildUuid,omitempty"`
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
	NamespaceName     string            `json:"namespaceName" validate:"required"`
	SearchPhrase      string            `json:"searchPhrase,omitempty"`
	APIIDToVersionMap map[string]string `json:"apiIdToVersionMap,omitempty"`
	GatewayVHosts     []string          `json:"gatewayVHosts,omitempty"`
	Limit             int               `json:"limit,omitempty"`
	SortOrder         string            `json:"sortOrder,omitempty"`
	LogType           string            `json:"logType,omitempty"`
}

// NamespaceLogsRequest represents the request body for namespace logs
type NamespaceLogsRequest struct {
	ComponentLogsRequest
	PodLabels map[string]string `json:"podLabels,omitempty"`
}

// MetricsRequest represents the request body for POST /api/metrics/component/usage API
type MetricsRequest struct {
	ComponentName   string `json:"componentName" validate:"required"`
	ComponentID     string `json:"componentId,omitempty"`
	EndTime         string `json:"endTime,omitempty"`
	EnvironmentName string `json:"environmentName,omitempty"`
	EnvironmentID   string `json:"environmentId" validate:"required"`
	NamespaceName   string `json:"namespaceName,omitempty"`
	StartTime       string `json:"startTime,omitempty"`
	ProjectName     string `json:"projectName,omitempty"`
	ProjectID       string `json:"projectId" validate:"required"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// GetBuildLogs handles POST /api/logs/build/{buildId}
func (h *Handler) GetBuildLogs(w http.ResponseWriter, r *http.Request) {
	buildID := httputil.GetPathParam(r, "buildId")
	if buildID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgBuildIDRequired)
		return
	}

	var req BuildLogsRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidRequestFormat)
		return
	}

	// AUTHORIZATION CHECK
	if h.authzPDP != nil {
		// Hierarchy fields are required for authorization but optional when authz is disabled
		if req.NamespaceName == "" || req.ProjectName == "" || req.ComponentName == "" {
			h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter,
				ErrorCodeMissingParameter, ErrorMsgMissingAuthHierarchy)
			return
		}
	}

	if err := observerAuthz.CheckAuthorization(
		r.Context(),
		h.logger,
		h.authzPDP,
		observerAuthz.ActionViewLogs,
		observerAuthz.ResourceTypeComponentWorkflowRun,
		buildID,
		authzcore.ResourceHierarchy{
			Namespace: req.NamespaceName,
			Project:   req.ProjectName,
			Component: req.ComponentName,
		},
	); err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			h.writeErrorResponse(w, http.StatusForbidden,
				ErrorTypeForbidden, ErrorCodeAuthForbidden, ErrorMsgAccessDenied)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			h.writeErrorResponse(w, http.StatusUnauthorized,
				ErrorTypeUnauthorized, ErrorCodeAuthUnauthorized, ErrorMsgUnauthorized)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable) {
			h.logger.Error(LogMsgAuthServiceUnavailableError, "error", err)
			h.writeErrorResponse(w, http.StatusInternalServerError,
				ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
			return
		}
		h.writeErrorResponse(w, http.StatusInternalServerError,
			ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
		return
	}

	// Validate times
	if err := validateTimes(req.StartTime, req.EndTime); err != nil {
		h.logger.Debug("Invalid/missing request parameters", "requestBody", req, "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, err.Error())
		return
	}

	// Set defaults
	if req.Limit == 0 {
		req.Limit = 100
	}
	if req.SortOrder == "" {
		req.SortOrder = ascendingSortOrder // Build logs are sorted in ascending order by default
	}

	// Build query parameters
	params := opensearch.BuildQueryParams{
		QueryParams: opensearch.QueryParams{
			StartTime: req.StartTime,
			EndTime:   req.EndTime,
			Limit:     req.Limit,
			SortOrder: req.SortOrder,
		},
		BuildID: buildID,
	}

	// Execute query
	ctx := r.Context()
	result, err := h.service.GetBuildLogs(ctx, params)
	if err != nil {
		h.logger.Error("Failed to get build logs", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToRetrieveLogs)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// GetWorkflowRunLogs handles POST /api/v1/workflow-runs/{runId}/logs
func (h *Handler) GetWorkflowRunLogs(w http.ResponseWriter, r *http.Request) {
	runID := httputil.GetPathParam(r, "runId")
	if runID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgWorkflowRunIDRequired)
		return
	}

	var req WorkflowRunLogsRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidRequestFormat)
		return
	}

	// AUTHORIZATION CHECK
	if h.authzPDP != nil {
		// Namespace name is required for authorization
		if req.NamespaceName == "" {
			h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter,
				ErrorCodeMissingParameter, ErrorMsgNamespaceNameRequired)
			return
		}
	}

	if err := observerAuthz.CheckAuthorization(
		r.Context(),
		h.logger,
		h.authzPDP,
		observerAuthz.ActionViewLogs,
		observerAuthz.ResourceTypeWorkflowRun,
		runID,
		authzcore.ResourceHierarchy{
			Namespace: req.NamespaceName,
		},
	); err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			h.writeErrorResponse(w, http.StatusForbidden,
				ErrorTypeForbidden, ErrorCodeAuthForbidden, ErrorMsgAccessDenied)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			h.writeErrorResponse(w, http.StatusUnauthorized,
				ErrorTypeUnauthorized, ErrorCodeAuthUnauthorized, ErrorMsgUnauthorized)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable) {
			h.logger.Error(LogMsgAuthServiceUnavailableError, "error", err)
			h.writeErrorResponse(w, http.StatusInternalServerError,
				ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
			return
		}
		h.writeErrorResponse(w, http.StatusInternalServerError,
			ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
		return
	}

	// Validate times
	if err := validateTimes(req.StartTime, req.EndTime); err != nil {
		h.logger.Debug("Invalid/missing request parameters", "requestBody", req, "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, err.Error())
		return
	}

	// Set defaults
	if req.Limit == 0 {
		req.Limit = 100
	}
	if req.SortOrder == "" {
		req.SortOrder = ascendingSortOrder // Workflow run logs are sorted in ascending order by default
	}

	// Build query parameters
	params := opensearch.WorkflowRunQueryParams{
		QueryParams: opensearch.QueryParams{
			StartTime:     req.StartTime,
			EndTime:       req.EndTime,
			Limit:         req.Limit,
			SortOrder:     req.SortOrder,
			NamespaceName: req.NamespaceName,
		},
		WorkflowRunID: runID,
	}

	// Execute query
	ctx := r.Context()
	result, err := h.service.GetWorkflowRunLogs(ctx, params)
	if err != nil {
		h.logger.Error("Failed to get workflow run logs", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToRetrieveLogs)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// GetComponentWorkflowRunLogs handles GET /api/v1/namespaces/{namespaceName}/projects/{projectName}/components/{componentName}/workflow-runs/{runName}/logs
func (h *Handler) GetComponentWorkflowRunLogs(w http.ResponseWriter, r *http.Request) {
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	runName := r.PathValue("runName")
	if runName == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgWorkflowRunIDRequired)
		return
	}

	// AUTHORIZATION CHECK
	if h.authzPDP != nil {
		// Hierarchy fields are required for authorization but optional when authz is disabled
		if namespaceName == "" || projectName == "" || componentName == "" {
			h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter,
				ErrorCodeMissingParameter, ErrorMsgMissingAuthHierarchy)
			return
		}

		if err := observerAuthz.CheckAuthorization(
			r.Context(),
			h.logger,
			h.authzPDP,
			observerAuthz.ActionViewLogs,
			observerAuthz.ResourceTypeComponentWorkflowRun,
			runName,
			authzcore.ResourceHierarchy{
				Namespace: namespaceName,
				Project:   projectName,
				Component: componentName,
			},
		); err != nil {
			if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
				h.writeErrorResponse(w, http.StatusForbidden,
					ErrorTypeForbidden, ErrorCodeAuthForbidden, ErrorMsgAccessDenied)
				return
			}
			if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
				h.writeErrorResponse(w, http.StatusUnauthorized,
					ErrorTypeUnauthorized, ErrorCodeAuthUnauthorized, ErrorMsgUnauthorized)
				return
			}
			if errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable) {
				h.logger.Error(LogMsgAuthServiceUnavailableError, "error", err)
				h.writeErrorResponse(w, http.StatusInternalServerError,
					ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
				return
			}
			h.writeErrorResponse(w, http.StatusInternalServerError,
				ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
			return
		}
	} else {
		h.logger.Debug("Authorization check skipped for component workflow run logs", "namespace", namespaceName,
			"project", projectName, "component", componentName, "run", runName)
	}

	// Get optional step query parameter
	step := r.URL.Query().Get("step")

	ctx := r.Context()
	logs, err := h.service.GetComponentWorkflowRunLogs(ctx, runName, step, defaultWorkflowRunLogsLimit)
	if err != nil {
		h.logger.Error("Failed to get component workflow run logs", "namespace", namespaceName, "project", projectName,
			"component", componentName, "run", runName, "step", step, "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToRetrieveLogs)
		return
	}

	h.writeJSON(w, http.StatusOK, logs)
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

	// AUTHORIZATION CHECK
	if h.authzPDP != nil {
		// Hierarchy fields are required for authorization but optional when authz is disabled
		if req.NamespaceName == "" || req.ProjectName == "" || req.ComponentName == "" {
			h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter,
				ErrorCodeMissingParameter, ErrorMsgMissingAuthHierarchy)
			return
		}
	}

	if err := observerAuthz.CheckAuthorization(
		r.Context(),
		h.logger,
		h.authzPDP,
		observerAuthz.ActionViewLogs,
		observerAuthz.ResourceTypeComponent,
		req.ComponentName,
		authzcore.ResourceHierarchy{
			Namespace: req.NamespaceName,
			Project:   req.ProjectName,
			Component: req.ComponentName,
		},
	); err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			h.writeErrorResponse(w, http.StatusForbidden,
				ErrorTypeForbidden, ErrorCodeAuthForbidden, ErrorMsgAccessDenied)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			h.writeErrorResponse(w, http.StatusUnauthorized,
				ErrorTypeUnauthorized, ErrorCodeAuthUnauthorized, ErrorMsgUnauthorized)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable) {
			h.logger.Error(LogMsgAuthServiceUnavailableError, "error", err)
			h.writeErrorResponse(w, http.StatusInternalServerError,
				ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
			return
		}
		h.writeErrorResponse(w, http.StatusInternalServerError,
			ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
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
	params := opensearch.ComponentQueryParams{
		QueryParams: opensearch.QueryParams{
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
			LogType:       opensearch.ExtractLogType(req.LogType),
		},
		BuildID:   req.BuildID,
		BuildUUID: req.BuildUUID,
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

	// AUTHORIZATION CHECK
	if h.authzPDP != nil {
		// Hierarchy fields are required for authorization but optional when authz is disabled
		if req.NamespaceName == "" || req.ProjectName == "" {
			h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter,
				ErrorCodeMissingParameter, "Namespace and Project names required for authorization")
			return
		}
	}

	if err := observerAuthz.CheckAuthorization(
		r.Context(),
		h.logger,
		h.authzPDP,
		observerAuthz.ActionViewLogs,
		observerAuthz.ResourceTypeProject,
		req.ProjectName,
		authzcore.ResourceHierarchy{
			Namespace: req.NamespaceName,
			Project:   req.ProjectName,
		},
	); err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			h.writeErrorResponse(w, http.StatusForbidden,
				ErrorTypeForbidden, ErrorCodeAuthForbidden, ErrorMsgAccessDenied)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			h.writeErrorResponse(w, http.StatusUnauthorized,
				ErrorTypeUnauthorized, ErrorCodeAuthUnauthorized, ErrorMsgUnauthorized)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable) {
			h.logger.Error(LogMsgAuthServiceUnavailableError, "error", err)
			h.writeErrorResponse(w, http.StatusInternalServerError,
				ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
			return
		}
		h.writeErrorResponse(w, http.StatusInternalServerError,
			ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
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
		LogType:       opensearch.ExtractLogType(req.LogType),
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
			LogType:      opensearch.ExtractLogType(req.LogType),
		},
		NamespaceName:     req.NamespaceName,
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

// GetNamespaceLogs handles POST /api/logs/namespace/{namespaceName}
func (h *Handler) GetNamespaceLogs(w http.ResponseWriter, r *http.Request) {
	namespaceName := httputil.GetPathParam(r, "namespaceName")
	if namespaceName == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgNamespaceNameRequired)
		return
	}

	var req NamespaceLogsRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidRequestFormat)
		return
	}

	// AUTHORIZATION CHECK
	if h.authzPDP != nil {
		// Hierarchy fields are required for authorization but optional when authz is disabled
		if req.NamespaceName == "" {
			h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter,
				ErrorCodeMissingParameter, "Namespace name required for authorization")
			return
		}
	}

	if err := observerAuthz.CheckAuthorization(
		r.Context(),
		h.logger,
		h.authzPDP,
		observerAuthz.ActionViewLogs,
		observerAuthz.ResourceTypeNamespace,
		req.NamespaceName,
		authzcore.ResourceHierarchy{
			Namespace: req.NamespaceName,
		},
	); err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			h.writeErrorResponse(w, http.StatusForbidden,
				ErrorTypeForbidden, ErrorCodeAuthForbidden, ErrorMsgAccessDenied)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			h.writeErrorResponse(w, http.StatusUnauthorized,
				ErrorTypeUnauthorized, ErrorCodeAuthUnauthorized, ErrorMsgUnauthorized)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable) {
			h.logger.Error(LogMsgAuthServiceUnavailableError, "error", err)
			h.writeErrorResponse(w, http.StatusInternalServerError,
				ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
			return
		}
		h.writeErrorResponse(w, http.StatusInternalServerError,
			ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
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
		EnvironmentID: req.EnvironmentID,
		Namespace:     req.Namespace,
		Versions:      req.Versions,
		VersionIDs:    req.VersionIDs,
		LogType:       opensearch.ExtractLogType(req.LogType),
		NamespaceName: namespaceName, // Add the namespace name from URL parameter
	}

	// Execute query
	ctx := r.Context()
	result, err := h.service.GetNamespaceLogs(ctx, params, req.PodLabels)
	if err != nil {
		h.logger.Error("Failed to get namespace logs", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToRetrieveLogs)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

func (h *Handler) GetTraces(w http.ResponseWriter, r *http.Request) {
	// Bind JSON request body
	var req opensearch.TracesRequestParams
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidRequestFormat)
		return
	}

	// AUTHORIZATION CHECK
	if h.authzPDP != nil {
		// Hierarchy fields are required for authorization but optional when authz is disabled
		if req.NamespaceName == "" || req.ProjectName == "" {
			h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter,
				ErrorCodeMissingParameter, "Namespace and Project names required for authorization")
			return
		}
	}

	if err := observerAuthz.CheckAuthorization(
		r.Context(),
		h.logger,
		h.authzPDP,
		observerAuthz.ActionViewTraces,
		observerAuthz.ResourceTypeProject,
		req.ProjectName,
		authzcore.ResourceHierarchy{
			Namespace: req.NamespaceName,
			Project:   req.ProjectName,
		},
	); err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			h.writeErrorResponse(w, http.StatusForbidden,
				ErrorTypeForbidden, ErrorCodeAuthForbidden, ErrorMsgAccessDenied)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			h.writeErrorResponse(w, http.StatusUnauthorized,
				ErrorTypeUnauthorized, ErrorCodeAuthUnauthorized, ErrorMsgUnauthorized)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable) {
			h.logger.Error(LogMsgAuthServiceUnavailableError, "error", err)
			h.writeErrorResponse(w, http.StatusInternalServerError,
				ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
			return
		}
		h.writeErrorResponse(w, http.StatusInternalServerError,
			ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
		return
	}

	// Input validations
	err := validateTimes(req.StartTime, req.EndTime)
	if err != nil {
		h.logger.Debug("Invalid/missing request parameters", "requestBody", req, "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, err.Error())
		return
	}

	err = validateComponentUIDs(req.ComponentUIDs)
	if err != nil {
		h.logger.Debug("Invalid/missing request parameter componentUids", "requestBody", req, "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, err.Error())
		return
	}

	err = validateSortOrder(&req.SortOrder)
	if err != nil {
		h.logger.Debug("Invalid sortOrder parameter", "requestBody", req, "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, err.Error())
		return
	}

	err = validateLimit(&req.Limit)
	if err != nil {
		h.logger.Debug("Invalid limit parameter", "requestBody", req, "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, err.Error())
		return
	}

	if req.ProjectUID == "" {
		h.logger.Debug("Missing required projectUid parameter", "requestBody", req)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, "Missing required projectUid parameter")
		return
	}

	err = validateTraceID(req.TraceID)
	if err != nil {
		h.logger.Debug("Invalid traceId parameter", "requestBody", req, "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, err.Error())
		return
	}

	// Execute query
	ctx := r.Context()
	result, err := h.service.GetTraces(ctx, req)
	if err != nil {
		h.logger.Error("Failed to get traces", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToRetrieveLogs)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// Health handles GET /health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.service.HealthCheck(ctx); err != nil {
		h.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
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

// GetComponentHTTPMetrics handles POST /api/metrics/component/http
func (h *Handler) GetComponentHTTPMetrics(w http.ResponseWriter, r *http.Request) {
	var req MetricsRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind metrics request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidRequestFormat)
		return
	}

	// AUTHORIZATION CHECK
	if h.authzPDP != nil {
		// Hierarchy fields are required for authorization but optional when authz is disabled
		if req.NamespaceName == "" || req.ProjectName == "" || req.ComponentName == "" {
			h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter,
				ErrorCodeMissingParameter, ErrorMsgMissingAuthHierarchy)
			return
		}
	}

	if err := observerAuthz.CheckAuthorization(
		r.Context(),
		h.logger,
		h.authzPDP,
		observerAuthz.ActionViewMetrics,
		observerAuthz.ResourceTypeComponent,
		req.ComponentName,
		authzcore.ResourceHierarchy{
			Namespace: req.NamespaceName,
			Project:   req.ProjectName,
			Component: req.ComponentName,
		},
	); err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			h.writeErrorResponse(w, http.StatusForbidden,
				ErrorTypeForbidden, ErrorCodeAuthForbidden, ErrorMsgAccessDenied)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			h.writeErrorResponse(w, http.StatusUnauthorized,
				ErrorTypeUnauthorized, ErrorCodeAuthUnauthorized, ErrorMsgUnauthorized)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable) {
			h.logger.Error(LogMsgAuthServiceUnavailableError, "error", err)
			h.writeErrorResponse(w, http.StatusInternalServerError,
				ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
			return
		}
		h.writeErrorResponse(w, http.StatusInternalServerError,
			ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
		return
	}

	var startTime, endTime time.Time
	var err error

	// Input validations
	err = validateTimes(req.StartTime, req.EndTime)
	if err != nil {
		h.logger.Debug("Invalid/missing request parameters", "requestBody", req, "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, err.Error())
		return
	}

	startTime, err = time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		h.logger.Error("Failed to parse start time", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidTimeFormat)
		return
	}

	endTime, err = time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		h.logger.Error("Failed to parse end time", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidTimeFormat)
		return
	}

	// Execute query
	ctx := r.Context()
	result, err := h.service.GetComponentHTTPMetrics(ctx, req.ComponentID, req.EnvironmentID, req.ProjectID, startTime, endTime)
	if err != nil {
		h.logger.Error("Failed to get component HTTP metrics", "error", err)
		h.writeJSON(w, http.StatusInternalServerError, map[string]string{
			"reason": "Internal error occurred while fetching one or more HTTP metrics",
		})
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// GetComponentResourceMetrics handles POST /api/metrics/component/usage
func (h *Handler) GetComponentResourceMetrics(w http.ResponseWriter, r *http.Request) {
	var req MetricsRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind metrics request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidRequestFormat)
		return
	}

	// AUTHORIZATION CHECK
	if h.authzPDP != nil {
		// Hierarchy fields are required for authorization but optional when authz is disabled
		if req.NamespaceName == "" || req.ProjectName == "" || req.ComponentName == "" {
			h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter,
				ErrorCodeMissingParameter, ErrorMsgMissingAuthHierarchy)
			return
		}
	}

	if err := observerAuthz.CheckAuthorization(
		r.Context(),
		h.logger,
		h.authzPDP,
		observerAuthz.ActionViewMetrics,
		observerAuthz.ResourceTypeComponent,
		req.ComponentName,
		authzcore.ResourceHierarchy{
			Namespace: req.NamespaceName,
			Project:   req.ProjectName,
			Component: req.ComponentName,
		},
	); err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			h.writeErrorResponse(w, http.StatusForbidden,
				ErrorTypeForbidden, ErrorCodeAuthForbidden, ErrorMsgAccessDenied)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			h.writeErrorResponse(w, http.StatusUnauthorized,
				ErrorTypeUnauthorized, ErrorCodeAuthUnauthorized, ErrorMsgUnauthorized)
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable) {
			h.logger.Error(LogMsgAuthServiceUnavailableError, "error", err)
			h.writeErrorResponse(w, http.StatusInternalServerError,
				ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
			return
		}
		h.writeErrorResponse(w, http.StatusInternalServerError,
			ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToAuthorize)
		return
	}

	var startTime, endTime time.Time
	var err error

	// Input validations
	err = validateTimes(req.StartTime, req.EndTime)
	if err != nil {
		h.logger.Debug("Invalid/missing request parameters", "requestBody", req, "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, err.Error())
		return
	}

	startTime, err = time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		h.logger.Error("Failed to parse start time", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidTimeFormat)
		return
	}

	endTime, err = time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		h.logger.Error("Failed to parse end time", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidTimeFormat)
		return
	}

	// Execute query
	ctx := r.Context()
	result, err := h.service.GetComponentResourceMetrics(ctx, req.ComponentID, req.EnvironmentID, req.ProjectID, startTime, endTime)
	if err != nil {
		h.logger.Error("Failed to get component resource metrics", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToRetrieveMetrics)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// UpsertAlertingRule handles PUT /api/alerting/rule/{sourceType}/{ruleName}
func (h *Handler) UpsertAlertingRule(w http.ResponseWriter, r *http.Request) {
	sourceType := httputil.GetPathParam(r, "sourceType")
	if sourceType == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgSourceTypeRequired)
		return
	}
	ruleName := httputil.GetPathParam(r, "ruleName")
	if ruleName == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgRuleNameRequired)
		return
	}
	var req types.AlertingRuleRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind alerting rule request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidRequestFormat)
		return
	}

	// Input validations
	err := validateAlertingRule(req)
	if err != nil {
		h.logger.Debug("Invalid alerting rule request", "requestBody", req, "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, err.Error())
		return
	}

	// Upsert the alerting rule
	ctx := r.Context()
	resp, err := h.service.UpsertAlertRule(ctx, sourceType, req)
	if err != nil {
		h.logger.Error("Failed to upsert alerting rule", "error", err, "ruleName", ruleName)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, "Failed to upsert alerting rule")
		return
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// DeleteAlertingRule handles DELETE /api/alerting/rule/{sourceType}/{ruleName}
func (h *Handler) DeleteAlertingRule(w http.ResponseWriter, r *http.Request) {
	sourceType := httputil.GetPathParam(r, "sourceType")
	if sourceType == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgSourceTypeRequired)
		return
	}
	ruleName := httputil.GetPathParam(r, "ruleName")
	if ruleName == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgRuleNameRequired)
		return
	}

	// Delete the alerting rule
	ctx := r.Context()
	resp, err := h.service.DeleteAlertRule(ctx, sourceType, ruleName)
	if err != nil {
		h.logger.Error("Failed to delete alerting rule", "error", err, "ruleName", ruleName)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, "Failed to delete alerting rule")
		return
	}

	// If rule was not found, return 404
	if resp.Status == "not_found" {
		h.writeJSON(w, http.StatusNotFound, resp)
		return
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// AlertingWebhook handles POST /api/alerting/webhook/{alertSource}
func (h *Handler) AlertingWebhook(w http.ResponseWriter, r *http.Request) {
	// Parse the webhook payload according to the alerting vendor and retrieve alert details
	ruleName, ruleNamespace, alertValue, timestamp, err := h.parseWebhookPayload(w, r)
	if err != nil {
		// Check if alert is not in firing state (ignore 'resolved' alerts from Prometheus)
		if err.Error() == "alert is not in firing state" {
			h.logger.Debug("Only alerts in firing state are processed. Non firing state (e.g. resolved alerts) are ignored.")
			h.writeJSON(w, http.StatusOK, map[string]interface{}{
				"message": "Alert ignored (not in firing state)",
			})
			return
		}
		h.logger.Error("Failed to parse webhook payload", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, "Failed to read request body")
		return
	}

	// Retrieve ObservabilityAlertRule CR details
	alertRule, err := h.service.GetObservabilityAlertRuleByName(r.Context(), ruleName, ruleNamespace)
	if err != nil {
		h.logger.Error("Failed to get ObservabilityAlertRule", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, "Failed to get ObservabilityAlertRule")
		return
	}

	// Enrich alert details with ObservabilityAlertRule CR details
	alertDetails, err := h.service.EnrichAlertDetails(alertRule, alertValue, timestamp)
	if err != nil {
		h.logger.Error("Failed to enrich alert details", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, "Failed to enrich alert details")
		return
	}

	// Store alert entry in logs backend
	alertID, err := h.service.StoreAlertEntry(r.Context(), alertDetails)
	if err != nil {
		h.logger.Error("Failed to store alert entry", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, "Failed to store alert entry")
		return
	}

	// Return success response acknowledging alert processing
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Alert acknowledged by OpenChoreo",
		"alertID": alertID,
	})

	// Send alert notification in background
	go func() {
		notifCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.service.SendAlertNotification(notifCtx, alertDetails); err != nil {
			h.logger.Warn("Failed to send alert notification", "error", err)
		}
	}()

	// Trigger AI RCA analysis in background if enabled
	if alertDetails.AlertAIRootCauseAnalysisEnabled {
		if isAIRCAEnabled() {
			go func() {
				h.logger.Info("AI RCA analysis triggered", "alertID", alertID)
				h.logger.Debug("AI RCA analysis details", "alertID", alertID, "alertDetails", alertDetails)
				h.service.TriggerRCAAnalysis(h.rcaServiceURL, alertID, alertDetails, alertRule)
			}()
		} else {
			h.logger.Info("AI RCA analysis not triggered", "alertID", alertID, "enableRCA", alertDetails.AlertAIRootCauseAnalysisEnabled)
		}
	}
}

// parseWebhookPayload reads and parses the JSON webhook payload
func (h *Handler) parseWebhookPayload(w http.ResponseWriter, r *http.Request) (ruleName string, ruleNamespace string, alertValue string, timestamp string, err error) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("Failed to read request body", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, "Failed to read request body")
		return "", "", "", "", err
	}

	if len(bodyBytes) == 0 {
		h.logger.Error("Alerting webhook received with empty request body")
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, "Empty request body")
		return "", "", "", "", fmt.Errorf("empty request body")
	}

	var requestBody map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &requestBody); err != nil {
		h.logger.Warn("Failed to parse webhook payload as JSON", "error", err, "body", string(bodyBytes))
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, "Failed to parse webhook payload as JSON")
		return "", "", "", "", err
	}

	// Debug log the request body for troubleshooting
	h.logger.Debug("Alert webhook received", "requestBody", string(bodyBytes))

	alertSource := httputil.GetPathParam(r, "alertSource")
	if alertSource == "" { // TODO: Add supported alert sources (e.g. opensearch, prometheus)
		h.logger.Error("Alert source is required")
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgAlertSourceRequired)
		return "", "", "", "", fmt.Errorf("alert source is required")
	}

	switch alertSource {
	case "opensearch":
		return h.service.ParseOpenSearchAlertPayload(requestBody)
	case "prometheus":
		return h.service.ParsePrometheusAlertPayload(requestBody)
	default:
		h.logger.Error("Unsupported alert source", "alertSource", alertSource)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, "Unsupported alert source")
		return "", "", "", "", fmt.Errorf("unsupported alert source")
	}
}
