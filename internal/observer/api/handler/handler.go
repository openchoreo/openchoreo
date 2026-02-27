// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"log/slog"
	"net/http"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// Handler contains the HTTP handlers for the new observer API (v1)
type Handler struct {
	logsService   *service.LogsService
	healthService *service.HealthService
	logger        *slog.Logger
	authzPDP      authzcore.PDP
}

// NewHandler creates a new handler instance for the new API
func NewHandler(
	logsService *service.LogsService,
	healthService *service.HealthService,
	logger *slog.Logger,
	authzPDP authzcore.PDP,
) *Handler {
	return &Handler{
		logsService:   logsService,
		healthService: healthService,
		logger:        logger,
		authzPDP:      authzPDP,
	}
}

// writeJSON writes JSON response and logs any error
func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	if err := httputil.WriteJSON(w, status, v); err != nil {
		h.logger.Error("Failed to write JSON response", "error", err)
	}
}

// writeErrorResponse writes a standardized error response for the new API
func (h *Handler) writeErrorResponse(w http.ResponseWriter, status int, title, errorCode, message string) {
	h.writeJSON(w, status, types.ErrorResponse{
		Title:     title,
		ErrorCode: errorCode,
		Message:   message,
	})
}
