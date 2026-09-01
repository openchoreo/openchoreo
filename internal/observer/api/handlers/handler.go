// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

// baseHandler holds state shared by Handler and InternalHandler.
//
// It used to carry writeJSON and writeErrorResponse helpers too. Those are gone:
// handlers no longer touch an http.ResponseWriter, they return a response object
// and the generated strict layer writes it. The equivalents now live in
// gen_adapters.go as jsonResponse and errorResponse.
type baseHandler struct {
	logger *slog.Logger
}

// Compile-time check that Handler implements the generated public strict server
// interface. Together with the internal one in alerts.go, this is what makes the
// specs authoritative: an operation added, removed or reshaped in
// openapi/observer-api.yaml becomes a build error here rather than a silent
// routing divergence.
var _ gen.StrictServerInterface = (*Handler)(nil)

// Handler contains the HTTP handlers for the public observer API (v1/v1alpha1).
// Routes are JWT-protected. Authorization is enforced by the service layer —
// pass authz-wrapped services (e.g. NewAlertIncidentServiceWithAuthz) rather
// than bare service instances.
type Handler struct {
	baseHandler
	healthService        service.HealthChecker
	logsService          service.LogsQuerier
	eventsService        service.EventsQuerier
	metricsService       service.MetricsQuerier
	alertIncidentService service.AlertIncidentService
	tracesService        service.TracesQuerier
	finOpsService        service.FinOpsQuerier
	oauthMetadata        OAuthMetadataConfig
}

// NewHandler creates a new public Handler instance.
func NewHandler(
	healthService service.HealthChecker,
	logsService service.LogsQuerier,
	eventsService service.EventsQuerier,
	metricsService service.MetricsQuerier,
	alertIncidentService service.AlertIncidentService,
	tracesService service.TracesQuerier,
	finOpsService service.FinOpsQuerier,
	oauthMetadata OAuthMetadataConfig,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		baseHandler:          baseHandler{logger: logger},
		healthService:        healthService,
		logsService:          logsService,
		eventsService:        eventsService,
		metricsService:       metricsService,
		alertIncidentService: alertIncidentService,
		tracesService:        tracesService,
		finOpsService:        finOpsService,
		oauthMetadata:        oauthMetadata,
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
