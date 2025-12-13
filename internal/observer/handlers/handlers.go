// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/httputil"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/internal/observer/service"
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
	ErrorMsgBuildIDRequired         = "Build ID is required"
	ErrorMsgComponentIDRequired     = "Component ID is required"
	ErrorMsgProjectIDRequired       = "Project ID is required"
	ErrorMsgOrganizationIDRequired  = "Organization ID is required"
	ErrorMsgAlertIDRequired         = "Alert ID is required"
	ErrorMsgInvalidRequestFormat    = "Invalid request format"
	ErrorMsgFailedToRetrieveLogs    = "Failed to retrieve logs"
	ErrorMsgFailedToRetrieveMetrics = "Failed to retrieve metrics"
	ErrorMsgFailedToRetrieveReports = "Failed to retrieve RCA reports"
	ErrorMsgReportNotFound          = "RCA report not found"
	ErrorMsgInvalidTimeFormat       = "Invalid time format"
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
	StartTime string `json:"startTime" validate:"required"`
	EndTime   string `json:"endTime" validate:"required"`
	Limit     int    `json:"limit,omitempty"`
	SortOrder string `json:"sortOrder,omitempty"`
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
	LogType       string   `json:"logType,omitempty"`
	BuildID       string   `json:"buildId,omitempty"`
	BuildUUID     string   `json:"buildUuid,omitempty"`
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
	ComponentID   string `json:"componentId,omitempty"`
	EndTime       string `json:"endTime,omitempty"`
	EnvironmentID string `json:"environmentId" validate:"required"`
	StartTime     string `json:"startTime,omitempty"`
	ProjectID     string `json:"projectId" validate:"required"`
}

// ProjectRCAReportsRequest represents the request body for getting RCA reports by project
type ProjectRCAReportsRequest struct {
	ComponentUIDs  []string `json:"componentUids,omitempty"`
	EnvironmentUID string   `json:"environmentUid,omitempty"`
	StartTime      string   `json:"startTime,omitempty"`
	EndTime        string   `json:"endTime,omitempty"`
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

// GetRCAReportsByProject handles POST /api/rca/project/{projectId}
func (h *Handler) GetRCAReportsByProject(w http.ResponseWriter, r *http.Request) {
	projectID := httputil.GetPathParam(r, "projectId")
	if projectID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeMissingParameter, ErrorCodeMissingParameter, ErrorMsgProjectIDRequired)
		return
	}

	var req ProjectRCAReportsRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, ErrorTypeInvalidRequest, ErrorCodeInvalidRequest, ErrorMsgInvalidRequestFormat)
		return
	}

	// Set defaults
	if req.Limit == 0 {
		req.Limit = 100
	}

	// Build query parameters
	params := opensearch.RCAReportQueryParams{
		ProjectUID:     projectID,
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
