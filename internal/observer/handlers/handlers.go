// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	choreoapis "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

const (
	defaultSortOrder = "desc"
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
	ErrorMsgComponentIDRequired       = "Component ID is required"
	ErrorMsgProjectIDRequired         = "Project ID is required"
	ErrorMsgOrganizationIDRequired    = "Organization ID is required"
	ErrorMsgEnvironmentIDRequired     = "Environment ID is required"
	ErrorMsgRuleNameRequired          = "Rule name is required"
	ErrorMsgSourceTypeRequired        = "Source type is required"
	ErrorMsgAlertIDRequired           = "Alert ID is required"
	ErrorMsgTimeRequired              = "Start time and end time are required"
	ErrorMsgInvalidRequestFormat      = "Invalid request format"
	ErrorMsgFailedToRetrieveLogs      = "Failed to retrieve logs"
	ErrorMsgFailedToRetrieveMetrics   = "Failed to retrieve metrics"
	ErrorMsgFailedToRetrieveReports   = "Failed to retrieve RCA reports"
	ErrorMsgReportNotFound            = "RCA report not found"
	ErrorMsgInvalidTimeFormat         = "Invalid time format"
	ErrorMsgAccessDenied              = "access denied due to insufficient permissions"
	ErrorMsgAuthServiceUnavailable    = "Authorization service temporarily unavailable"
	ErrorMsgFailedToAuthorize         = "Failed to authorize request"
	ErrorMsgUnauthorized              = "Unauthorized request"
	ErrorMsgMissingAuthHierarchy      = "missing required fields for authorization"
	LogMsgAuthServiceUnavailableError = "Authorization service unavailable or timed out"
)

// Handler contains the HTTP handlers for the logging API
type Handler struct {
	service               *service.LoggingService
	logger                *slog.Logger
	authzPDP              authzcore.PDP
	alertingWebhookSecret string
	rcaServiceURL         string
}

// NewHandler creates a new handler instance
func NewHandler(service *service.LoggingService, logger *slog.Logger, authzPDP authzcore.PDP, alertingWebhookSecret, rcaServiceURL string) *Handler {
	return &Handler{
		service:               service,
		logger:                logger,
		authzPDP:              authzPDP,
		alertingWebhookSecret: alertingWebhookSecret,
		rcaServiceURL:         rcaServiceURL,
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
	OrgName       string `json:"orgName,omitempty"`
	ProjectName   string `json:"projectName,omitempty"`
	StartTime     string `json:"startTime" validate:"required"`
	EndTime       string `json:"endTime" validate:"required"`
	Limit         int    `json:"limit,omitempty"`
	SortOrder     string `json:"sortOrder,omitempty"`
}

// ComponentLogsRequest represents the request body for component logs
type ComponentLogsRequest struct {
	ComponentName   string   `json:"componentName,omitempty"`
	OrgName         string   `json:"orgName,omitempty"`
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
	OrganizationID    string            `json:"organizationId" validate:"required"`
	SearchPhrase      string            `json:"searchPhrase,omitempty"`
	APIIDToVersionMap map[string]string `json:"apiIdToVersionMap,omitempty"`
	GatewayVHosts     []string          `json:"gatewayVHosts,omitempty"`
	Limit             int               `json:"limit,omitempty"`
	SortOrder         string            `json:"sortOrder,omitempty"`
	LogType           string            `json:"logType,omitempty"`
}

// OrganizationLogsRequest represents the request body for organization logs
type OrganizationLogsRequest struct {
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
	OrgName         string `json:"orgName,omitempty"`
	StartTime       string `json:"startTime,omitempty"`
	ProjectName     string `json:"projectName,omitempty"`
	ProjectID       string `json:"projectId" validate:"required"`
}

// ProjectRCAReportsRequest represents the request body for getting RCA reports by project
type ProjectRCAReportsRequest struct {
	ComponentUIDs  []string `json:"componentUids,omitempty"`
	EnvironmentUID string   `json:"environmentUid"`
	StartTime      string   `json:"startTime"`
	EndTime        string   `json:"endTime"`
	Status         string   `json:"status,omitempty"`
	Limit          int      `json:"limit,omitempty"`
}

// RCAReportSummary represents a summary entry in the list of RCA reports
type RCAReportSummary struct {
	AlertID    string `json:"alertId"`
	ProjectUID string `json:"projectUid"`
	ReportID   string `json:"reportId"`
	Timestamp  string `json:"timestamp"`
	Summary    string `json:"summary"`
	Status     string `json:"status"`
}

// RCAReportsResponse represents the response for listing RCA reports
type RCAReportsResponse struct {
	Reports    []RCAReportSummary `json:"reports"`
	TotalCount int                `json:"totalCount"`
	TookMs     int                `json:"tookMs"`
}

// RCAReportDetailed represents a full detailed RCA report with version information and arbitrary JSON data
type RCAReportDetailed struct {
	AlertID           string `json:"alertId"`
	ProjectUID        string `json:"projectUid"`
	ReportVersion     int    `json:"reportVersion"`
	ReportID          string `json:"reportId"`
	Timestamp         string `json:"timestamp"`
	Status            string `json:"status"`
	AvailableVersions []int  `json:"availableVersions"`
	// Additional arbitrary fields will be included via custom marshaling if needed
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
		if req.OrgName == "" || req.ProjectName == "" || req.ComponentName == "" {
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
			Organization: req.OrgName,
			Project:      req.ProjectName,
			Component:    req.ComponentName,
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
		req.SortOrder = "asc" // Build logs are sorted in ascending order by default
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
		if req.OrgName == "" || req.ProjectName == "" || req.ComponentName == "" {
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
			Organization: req.OrgName,
			Project:      req.ProjectName,
			Component:    req.ComponentName,
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
		if req.OrgName == "" || req.ProjectName == "" {
			h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter,
				ErrorCodeMissingParameter, "Organization and Project names required for authorization")
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
			Organization: req.OrgName,
			Project:      req.ProjectName,
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

	// AUTHORIZATION CHECK
	if h.authzPDP != nil {
		// Hierarchy fields are required for authorization but optional when authz is disabled
		if req.OrgName == "" {
			h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter,
				ErrorCodeMissingParameter, "Organization name required for authorization")
			return
		}
	}

	if err := observerAuthz.CheckAuthorization(
		r.Context(),
		h.logger,
		h.authzPDP,
		observerAuthz.ActionViewLogs,
		observerAuthz.ResourceTypeOrg,
		req.OrgName,
		authzcore.ResourceHierarchy{
			Organization: req.OrgName,
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
		LogType:        opensearch.ExtractLogType(req.LogType),
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
		if req.OrgName == "" || req.ProjectName == "" {
			h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter,
				ErrorCodeMissingParameter, "Organization and Project names required for authorization")
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
			Organization: req.OrgName,
			Project:      req.ProjectName,
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
		if req.OrgName == "" || req.ProjectName == "" || req.ComponentName == "" {
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
			Organization: req.OrgName,
			Project:      req.ProjectName,
			Component:    req.ComponentName,
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
		if req.OrgName == "" || req.ProjectName == "" || req.ComponentName == "" {
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
			Organization: req.OrgName,
			Project:      req.ProjectName,
			Component:    req.ComponentName,
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
	resp, err := h.service.UpsertAlertRule(ctx, sourceType, ruleName, req)
	if err != nil {
		h.logger.Error("Failed to upsert alerting rule", "error", err, "ruleName", req.Metadata.Name)
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

// AlertingWebhook handles POST /api/alerting/webhook/{secret}
func (h *Handler) AlertingWebhook(w http.ResponseWriter, r *http.Request) {
	// Validate the shared webhook secret to ensure the request originates from a trusted source.
	secret := httputil.GetPathParam(r, "secret")
	if secret == "" || secret != h.alertingWebhookSecret {
		h.logger.Warn("Received alerting webhook with invalid or missing secret")
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	// Read the request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("Failed to read request body", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, "Failed to read request body")
		return
	}

	// Parse the webhook payload as JSON
	if len(bodyBytes) == 0 {
		h.logger.Warn("Alerting webhook received with empty request body")
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, "Failed to read request body")
		return
	}

	var requestBody map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &requestBody); err != nil {
		h.logger.Warn("Failed to parse webhook payload as JSON", "error", err, "body", string(bodyBytes))
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, "Failed to parse webhook payload as JSON")
		return
	}

	/* Message format
	{
		"alertValue": 108, (value > threshold)
		"componentUid": "41f02f7c-cead-477e-a4ca-3678523ab1d5",
		"enableAiRootCauseAnalysis": false,
		"notificationChannel": "default-smp-channel",
		"environmentUid": "46ba778e-4e58-4557-8df4-654c8e1e92d1",
		"projectUid": "9ec65f73-c507-4f83-9894-fbc57366527a",
		"ruleName": "requests-log-alert-rule",
		"timestamp": "2025-12-21T14:51:08.592Z"
	}
	*/
	h.logger.Debug("Successfully parsed webhook payload.")

	ctx := r.Context()

	var alertRule *choreoapis.ObservabilityAlertRule
	ruleName, _ := requestBody["ruleName"].(string)
	componentUID, _ := requestBody["componentUid"].(string)
	projectUID, _ := requestBody["projectUid"].(string)
	environmentUID, _ := requestBody["environmentUid"].(string)

	// TODO: Remove label selectors and use direct Get by NamespacedName
	if ruleName != "" && componentUID != "" && projectUID != "" && environmentUID != "" {
		var err error
		alertRule, err = h.service.GetObservabilityAlertRuleByName(ctx, ruleName, componentUID, projectUID, environmentUID)
		if err != nil {
			h.logger.Warn("Failed to fetch ObservabilityAlertRule", "ruleName", ruleName, "error", err)
		}
	}

	specRuleName := ruleName
	if alertRule != nil {
		specRuleName = alertRule.Spec.Name
	}

	// Send alert notification
	if err := h.service.SendAlertNotification(ctx, requestBody, specRuleName); err != nil {
		h.logger.Error("Failed to send alert notification", "error", err)
	}

	// Store alert entry in logs backend
	alertID, err := h.service.StoreAlertEntry(ctx, requestBody, specRuleName)
	if err != nil {
		h.logger.Error("Failed to store alert entry", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, "Failed to store alert entry")
		return
	}

	// Trigger AI RCA analysis if enabled
	if enableRCA, ok := requestBody["enableAiRootCauseAnalysis"].(bool); ok && enableRCA {
		if isAIRCAEnabled() {
			h.service.TriggerRCAAnalysis(ctx, h.rcaServiceURL, alertID, requestBody, alertRule)
		}
	}

	// Return the alertID
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Alert notification sent",
		"alertID": alertID,
	})
}

// GetRCAReportsByProject handles POST /api/rca/project/{projectUid}
func (h *Handler) GetRCAReportsByProject(w http.ResponseWriter, r *http.Request) {
	projectUID := httputil.GetPathParam(r, "projectUid")
	if projectUID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgProjectIDRequired)
		return
	}

	var req ProjectRCAReportsRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidRequestFormat)
		return
	}

	// Validate required fields
	if req.EnvironmentUID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgEnvironmentIDRequired)
		return
	}
	if req.StartTime == "" || req.EndTime == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgTimeRequired)
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

	// Build query parameters
	params := opensearch.RCAReportQueryParams{
		ProjectUID:     projectUID,
		ComponentUIDs:  req.ComponentUIDs,
		EnvironmentUID: req.EnvironmentUID,
		StartTime:      req.StartTime,
		EndTime:        req.EndTime,
		Status:         req.Status,
		Limit:          req.Limit,
		SortOrder:      "desc",
	}

	// Call service to retrieve reports
	result, err := h.service.GetRCAReportsByProject(r.Context(), params)
	if err != nil {
		h.logger.Error("Failed to retrieve RCA reports", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgFailedToRetrieveReports)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// GetRCAReportByAlert handles GET /api/rca-reports/alert/{alertId}?version=N
func (h *Handler) GetRCAReportByAlert(w http.ResponseWriter, r *http.Request) {
	alertID := httputil.GetPathParam(r, "alertId")
	if alertID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgAlertIDRequired)
		return
	}

	// Parse optional version query parameter
	var version *int
	if versionStr := r.URL.Query().Get("version"); versionStr != "" {
		var v int
		if _, err := fmt.Sscanf(versionStr, "%d", &v); err != nil || v < 1 {
			h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, "Invalid version parameter")
			return
		}
		version = &v
	}

	// Build query parameters
	params := opensearch.RCAReportByAlertQueryParams{
		AlertID: alertID,
		Version: version,
	}

	// Call service to retrieve report
	result, err := h.service.GetRCAReportByAlert(r.Context(), params)
	if err != nil {
		h.logger.Error("Failed to retrieve RCA report", "error", err)
		h.writeErrorResponse(w, http.StatusNotFound, ErrorTypeInternalError, ErrorCodeInternalError, ErrorMsgReportNotFound)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}
