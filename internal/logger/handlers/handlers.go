package handlers

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/openchoreo/openchoreo/internal/logger/opensearch"
	"github.com/openchoreo/openchoreo/internal/logger/service"
)

const (
	defaultSortOrder = "desc"
)

// Handler contains the HTTP handlers for the logging API
type Handler struct {
	service *service.LoggingService
	logger  *zap.Logger
}

// NewHandler creates a new handler instance
func NewHandler(service *service.LoggingService, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// ComponentLogsRequest represents the request body for component logs
type ComponentLogsRequest struct {
	StartTime     string   `json:"start_time" validate:"required"`
	EndTime       string   `json:"end_time" validate:"required"`
	EnvironmentID string   `json:"environment_id" validate:"required"`
	Namespace     string   `json:"namespace" validate:"required"`
	SearchPhrase  string   `json:"search_phrase,omitempty"`
	LogLevels     []string `json:"log_levels,omitempty"`
	Versions      []string `json:"versions,omitempty"`
	VersionIDs    []string `json:"version_ids,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	SortOrder     string   `json:"sort_order,omitempty"`
}

// ProjectLogsRequest represents the request body for project logs
type ProjectLogsRequest struct {
	ComponentLogsRequest
	ComponentIDs []string `json:"component_ids,omitempty"`
}

// GatewayLogsRequest represents the request body for gateway logs
type GatewayLogsRequest struct {
	StartTime         string            `json:"start_time" validate:"required"`
	EndTime           string            `json:"end_time" validate:"required"`
	OrganizationID    string            `json:"organization_id" validate:"required"`
	SearchPhrase      string            `json:"search_phrase,omitempty"`
	APIIDToVersionMap map[string]string `json:"api_id_to_version_map,omitempty"`
	GatewayVHosts     []string          `json:"gateway_vhosts,omitempty"`
	Limit             int               `json:"limit,omitempty"`
	SortOrder         string            `json:"sort_order,omitempty"`
}

// OrganizationLogsRequest represents the request body for organization logs
type OrganizationLogsRequest struct {
	ComponentLogsRequest
	PodLabels map[string]string `json:"pod_labels,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// GetComponentLogs handles GET /api/v2/logs/component/:componentId
func (h *Handler) GetComponentLogs(c echo.Context) error {
	componentID := c.Param("componentId")
	if componentID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_parameter",
			Code:    "OBS-L-10",
			Message: "Component ID is required",
		})
	}

	var req ComponentLogsRequest
	if err := c.Bind(&req); err != nil {
		h.logger.Error("Failed to bind request", zap.Error(err))
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Code:    "OBS-L-12",
			Message: "Invalid request format",
		})
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
	ctx := c.Request().Context()
	result, err := h.service.GetComponentLogs(ctx, params)
	if err != nil {
		h.logger.Error("Failed to get component logs", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Code:    "OBS-L-25",
			Message: "Failed to retrieve logs",
		})
	}

	return c.JSON(http.StatusOK, result)
}

// GetProjectLogs handles GET /api/v2/logs/project/:projectId
func (h *Handler) GetProjectLogs(c echo.Context) error {
	projectID := c.Param("projectId")
	if projectID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_parameter",
			Code:    "OBS-L-10",
			Message: "Project ID is required",
		})
	}

	var req ProjectLogsRequest
	if err := c.Bind(&req); err != nil {
		h.logger.Error("Failed to bind request", zap.Error(err))
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Code:    "OBS-L-12",
			Message: "Invalid request format",
		})
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
	ctx := c.Request().Context()
	result, err := h.service.GetProjectLogs(ctx, params, req.ComponentIDs)
	if err != nil {
		h.logger.Error("Failed to get project logs", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Code:    "OBS-L-25",
			Message: "Failed to retrieve logs",
		})
	}

	return c.JSON(http.StatusOK, result)
}

// GetGatewayLogs handles GET /api/v2/logs/gateway
func (h *Handler) GetGatewayLogs(c echo.Context) error {
	var req GatewayLogsRequest
	if err := c.Bind(&req); err != nil {
		h.logger.Error("Failed to bind request", zap.Error(err))
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Code:    "OBS-L-12",
			Message: "Invalid request format",
		})
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
	ctx := c.Request().Context()
	result, err := h.service.GetGatewayLogs(ctx, params)
	if err != nil {
		h.logger.Error("Failed to get gateway logs", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Code:    "OBS-L-25",
			Message: "Failed to retrieve logs",
		})
	}

	return c.JSON(http.StatusOK, result)
}

// GetOrganizationLogs handles GET /api/v2/logs/org/:orgId
func (h *Handler) GetOrganizationLogs(c echo.Context) error {
	orgID := c.Param("orgId")
	if orgID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_parameter",
			Code:    "OBS-L-10",
			Message: "Organization ID is required",
		})
	}

	var req OrganizationLogsRequest
	if err := c.Bind(&req); err != nil {
		h.logger.Error("Failed to bind request", zap.Error(err))
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Code:    "OBS-L-12",
			Message: "Invalid request format",
		})
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
	ctx := c.Request().Context()
	result, err := h.service.GetOrganizationLogs(ctx, params, req.PodLabels)
	if err != nil {
		h.logger.Error("Failed to get organization logs", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Code:    "OBS-L-25",
			Message: "Failed to retrieve logs",
		})
	}

	return c.JSON(http.StatusOK, result)
}

// Health handles GET /health
func (h *Handler) Health(c echo.Context) error {
	ctx := c.Request().Context()
	if err := h.service.HealthCheck(ctx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"status": "unhealthy",
			"error":  err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
