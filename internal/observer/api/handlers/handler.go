// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/api/internalgen"
	observermiddleware "github.com/openchoreo/openchoreo/internal/observer/middleware"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

// baseHandler holds state shared by Handler and InternalHandler.
type baseHandler struct {
	logger *slog.Logger
}

// Compile-time check that Handler implements the generated public strict server
// interface.
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
	insightsService      service.InsightsService
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
	insightsService service.InsightsService,
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
		insightsService:      insightsService,
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

// ObserverMiddlewareOptions carries the dependencies ObserverMiddlewares needs.
type ObserverMiddlewareOptions struct {
	Logger *slog.Logger
	// AuthMiddleware is auth.OpenAPIAuth(jwtMiddleware, gen.BearerAuthScopes)
	// in production. Must not be nil.
	AuthMiddleware func(http.Handler) http.Handler
}

// ObserverMiddlewares returns the ordered middleware chain for the generated
// public OpenAPI routes.
//
// oapi-codegen applies these last-to-first, so the last entry is outermost:
//
//	logger → recovery → auth → contentType → handler
//
// contentType sits innermost, inside auth, so an unauthenticated caller cannot
// probe it.
//
// Auth is not applied to a hand-picked set of routes. It wraps every generated
// route, and auth.OpenAPIAuth decides per request by reading the scopes context
// key the generated wrapper sets. Which routes are public is therefore decided
// by the spec: /health and /.well-known/oauth-protected-resource carry
// `security: []`, and nothing else does. TestObserverMiddlewaresLeaveHealthPublic
// and TestGetOAuthProtectedResourceMetadata_NeedsNoToken drive the real
// auth.OpenAPIAuth against both.
//
// This is the single definition of the chain — main.go supplies dependencies but
// owns no ordering.
func ObserverMiddlewares(opts ObserverMiddlewareOptions) ([]gen.MiddlewareFunc, error) {
	if opts.AuthMiddleware == nil {
		return nil, errors.New("observer: ObserverMiddlewareOptions.AuthMiddleware must not be nil")
	}

	return []gen.MiddlewareFunc{
		RequireJSONContentType(opts.Logger),
		opts.AuthMiddleware,
		observermiddleware.Recovery(opts.Logger),
		observermiddleware.Logger(opts.Logger),
	}, nil
}

// InternalMiddlewareOptions carries the dependencies InternalMiddlewares needs.
type InternalMiddlewareOptions struct {
	Logger *slog.Logger
}

// InternalMiddlewares returns the ordered middleware chain for the generated
// internal OpenAPI routes (port 8081).
//
// oapi-codegen applies these last-to-first, so the last entry is outermost:
//
//	logger → recovery → handler
//
// There is deliberately no auth middleware here. The internal API declares no
// security scheme, because the internal port has no JWT layer and the
// ObservabilityAlertRule controller that drives alert rule CRUD sends no
// Authorization header. Do not add auth here without the controller-side token
// work that must accompany it.
//
// This is the single definition of the chain — main.go supplies dependencies but
// owns no ordering.
func InternalMiddlewares(opts InternalMiddlewareOptions) []internalgen.MiddlewareFunc {
	return []internalgen.MiddlewareFunc{
		observermiddleware.Recovery(opts.Logger),
		observermiddleware.Logger(opts.Logger),
	}
}
