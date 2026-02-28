// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"
	"net/http"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

// Handler contains the HTTP handlers for the new observer API (v1)
type Handler struct {
	healthService  *service.HealthService
	logsService    *service.LogsService
	metricsService *service.MetricsService
	logger         *slog.Logger
	authzPDP       authzcore.PDP
}

// NewHandler creates a new handler instance for the new API
func NewHandler(
	healthService *service.HealthService,
	logsService *service.LogsService,
	metricsService *service.MetricsService,
	logger *slog.Logger,
	authzPDP authzcore.PDP,
) *Handler {
	return &Handler{
		healthService:  healthService,
		logsService:    logsService,
		metricsService: metricsService,
		logger:         logger,
		authzPDP:       authzPDP,
	}
}

// writeJSON writes JSON response and logs any error
func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	if err := httputil.WriteJSON(w, status, v); err != nil {
		h.logger.Error("Failed to write JSON response", "error", err)
	}
}

// writeErrorResponse writes a standardized error response for the new API
func (h *Handler) writeErrorResponse(
	w http.ResponseWriter,
	status int,
	title gen.ErrorResponseTitle,
	errorCode string,
	message string,
) {
	h.writeJSON(w, status, gen.ErrorResponse{
		Title:     &title,
		ErrorCode: &errorCode,
		Message:   &message,
	})
}
