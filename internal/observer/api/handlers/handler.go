// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

// baseHandler holds helpers shared by Handler and InternalHandler.
type baseHandler struct {
	logger *slog.Logger
}

// writeJSON writes JSON response and logs any error.
func (b *baseHandler) writeJSON(w http.ResponseWriter, status int, v any) {
	if err := httputil.WriteJSON(w, status, v); err != nil {
		b.logger.Error("Failed to write JSON response", "error", err)
	}
}

// writeErrorResponse writes a standardized error response.
func (b *baseHandler) writeErrorResponse(
	w http.ResponseWriter,
	status int,
	title gen.ErrorResponseTitle,
	errorCode string,
	message string,
) {
	b.writeJSON(w, status, gen.ErrorResponse{
		Title:     &title,
		ErrorCode: &errorCode,
		Message:   &message,
	})
}

// Handler contains the HTTP handlers for the public observer API (v1/v1alpha1).
// Routes are JWT-protected. Authorization is enforced by the service layer —
// pass authz-wrapped services (e.g. NewAlertIncidentServiceWithAuthz) rather
// than bare service instances.
type Handler struct {
	baseHandler
	healthService        service.HealthChecker
	logsService          service.LogsQuerier
	metricsService       service.MetricsQuerier
	alertIncidentService service.AlertIncidentService
	tracesService        service.TracesQuerier
}

// NewHandler creates a new public Handler instance.
func NewHandler(
	healthService service.HealthChecker,
	logsService service.LogsQuerier,
	metricsService service.MetricsQuerier,
	alertIncidentService service.AlertIncidentService,
	tracesService service.TracesQuerier,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		baseHandler:          baseHandler{logger: logger},
		healthService:        healthService,
		logsService:          logsService,
		metricsService:       metricsService,
		alertIncidentService: alertIncidentService,
		tracesService:        tracesService,
	}
}

// InternalHandler contains the HTTP handlers that run on the internal port (8081)
// without JWT authentication. It manages alert rules and processes incoming webhooks.
type InternalHandler struct {
	baseHandler
	alertService service.AlertRuleService
}

// NewInternalHandler creates a new InternalHandler instance.
func NewInternalHandler(
	alertService service.AlertRuleService,
	logger *slog.Logger,
) *InternalHandler {
	return &InternalHandler{
		baseHandler:  baseHandler{logger: logger},
		alertService: alertService,
	}
}
