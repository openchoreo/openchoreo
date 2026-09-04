// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/api/internalgen"
	observeraudit "github.com/openchoreo/openchoreo/internal/observer/audit"
	observermiddleware "github.com/openchoreo/openchoreo/internal/observer/middleware"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/server/middleware"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	apilogger "github.com/openchoreo/openchoreo/internal/server/middleware/logger"
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

// newAuditMiddleware builds an audit.Middleware for one of observer's two
// generated specs, from the subset of the audit table that spec declares.
// The filter is required: BuildPatternMap errors on an operationId it can't
// resolve to a route, so passing the whole table would fail startup.
func newAuditMiddleware(
	logger *slog.Logger,
	getSwagger func() (*openapi3.T, error),
	emitter *audit.Emitter,
	enabled bool,
) (*audit.Middleware, error) {
	swagger, err := getSwagger()
	if err != nil {
		return nil, fmt.Errorf("audit: failed to load OpenAPI spec: %w", err)
	}
	return audit.NewMiddleware(logger, observeraudit.OperationsIn(swagger), getSwagger, emitter, enabled)
}

// ObserverMiddlewareOptions carries the dependencies ObserverMiddlewares needs.
type ObserverMiddlewareOptions struct {
	Logger *slog.Logger
	// AuthMiddleware is auth.OpenAPIAuth(jwtMiddleware, gen.BearerAuthScopes)
	// in production. Must not be nil.
	AuthMiddleware func(http.Handler) http.Handler
	// AuditEmitter is shared with InternalMiddlewares and the /mcp chain so one
	// policy applies across every surface. Must not be nil.
	AuditEmitter *audit.Emitter
	AuditEnabled bool
}

// ObserverMiddlewares returns the ordered middleware chain for the generated
// public OpenAPI routes, mirroring openchoreo-api's OpenAPIMiddlewares.
//
// oapi-codegen applies these last-to-first, so the last entry is outermost:
//
//	logger → recovery → unauthenticatedAudit → auth → audit → contentType → handler
//
// audit sits inside auth so SubjectContext is already populated for it: it
// captures its context before calling next, and the JWT middleware populates a
// child context that never propagates back. Outside auth, every event would
// emit as anonymous with nothing failing.
//
// unauthenticatedAudit sits outside auth — the only position that can see a
// request auth itself rejects, since auth short-circuits and never calls next.
// contentType stays innermost so an unauthenticated caller cannot probe it.
//
// Auth wraps every generated route; auth.OpenAPIAuth decides per request from
// the scopes context key the generated wrapper sets, so which routes are
// public is decided by the spec (`security: []` on /health and
// /.well-known/oauth-protected-resource) rather than by this middleware list.
//
// This is the single definition of the chain — main.go supplies dependencies
// but owns no ordering. Errors rather than panics, so main can report a
// misconfiguration through its usual startup-failure path.
func ObserverMiddlewares(opts ObserverMiddlewareOptions) ([]gen.MiddlewareFunc, error) {
	if opts.AuthMiddleware == nil {
		return nil, errors.New("observer: ObserverMiddlewareOptions.AuthMiddleware must not be nil")
	}
	if opts.AuditEmitter == nil {
		return nil, errors.New("observer: ObserverMiddlewareOptions.AuditEmitter must not be nil")
	}

	auditMw, err := newAuditMiddleware(opts.Logger, gen.GetSwagger, opts.AuditEmitter, opts.AuditEnabled)
	if err != nil {
		return nil, err
	}
	unauthenticatedAuditMw := audit.NewUnauthenticatedMiddleware(
		opts.AuditEmitter, audit.OriginAPI, opts.AuditEnabled)

	return []gen.MiddlewareFunc{
		RequireJSONContentType(opts.Logger),
		auditMw.Handler,
		opts.AuthMiddleware,
		unauthenticatedAuditMw,
		observermiddleware.Recovery(opts.Logger),
		apilogger.Middleware(opts.Logger),
	}, nil
}

// MCPMiddlewareOptions carries the dependencies MCPMiddlewares needs. All
// three must be non-nil.
type MCPMiddlewareOptions struct {
	// Auth401 is mcpmiddleware.Auth401Interceptor in production.
	Auth401 func(http.Handler) http.Handler
	// JWTAuth is the same JWT middleware the public REST chain wraps.
	JWTAuth      func(http.Handler) http.Handler
	AuditEmitter *audit.Emitter
	AuditEnabled bool
}

// MCPMiddlewares returns the middlewares to group onto /mcp, on top of the
// logger and recovery the base route builder already carries.
//
// These are middleware.Chain-ordered — first entry outermost, the opposite of
// the generated servers' slices. cmd/observer holds both conventions, which is
// why this ordering lives here rather than inline at the call site:
//
//	logger → recovery → unauthenticatedAudit → auth401 → jwt → handler
//
// unauthenticatedAudit sits outside jwt for the same reason as in
// ObserverMiddlewares. Reverse the two and an MCP token rejection silently
// emits nothing.
//
// The OriginMCP instance is separate from ObserverMiddlewares' OriginAPI one:
// sharing would misattribute MCP rejections to REST, and nesting would
// double-emit. They never stack, since /mcp is registered on the base mux.
//
// No operation-level audit middleware: observer registers no mutating MCP
// tools (see internal/observer/audit's MCPToolNames).
func MCPMiddlewares(opts MCPMiddlewareOptions) ([]middleware.Middleware, error) {
	if opts.Auth401 == nil {
		return nil, errors.New("observer: MCPMiddlewareOptions.Auth401 must not be nil")
	}
	if opts.JWTAuth == nil {
		return nil, errors.New("observer: MCPMiddlewareOptions.JWTAuth must not be nil")
	}
	if opts.AuditEmitter == nil {
		return nil, errors.New("observer: MCPMiddlewareOptions.AuditEmitter must not be nil")
	}

	return []middleware.Middleware{
		audit.NewUnauthenticatedMiddleware(opts.AuditEmitter, audit.OriginMCP, opts.AuditEnabled),
		opts.Auth401,
		opts.JWTAuth,
	}, nil
}

// InternalMiddlewareOptions carries the dependencies InternalMiddlewares needs.
type InternalMiddlewareOptions struct {
	Logger *slog.Logger
	// AuditEmitter is the same emitter ObserverMiddlewares receives. Must not
	// be nil.
	AuditEmitter *audit.Emitter
	AuditEnabled bool
}

// InternalMiddlewares returns the ordered middleware chain for the generated
// internal OpenAPI routes (port 8081).
//
// oapi-codegen applies these last-to-first, so the last entry is outermost:
//
//	logger → recovery → audit → handler
//
// There is deliberately no auth middleware here. The internal API declares no
// security scheme, because the internal port has no JWT layer and the
// ObservabilityAlertRule controller that drives alert rule CRUD sends no
// Authorization header. Do not add auth here without the controller-side token
// work that must accompany it.
//
// Audit is wired even though every operation here is exempted today — with no
// auth there is no actor to record — so coverage becomes automatic if an
// exemption lifts. Until then OperationsIn resolves to an empty set and the
// middleware is a pass-through. No unauthenticated-audit middleware, since
// without auth there is no 401 to observe.
//
// This is the single definition of the chain — main.go supplies dependencies
// but owns no ordering.
func InternalMiddlewares(opts InternalMiddlewareOptions) ([]internalgen.MiddlewareFunc, error) {
	if opts.AuditEmitter == nil {
		return nil, errors.New("observer: InternalMiddlewareOptions.AuditEmitter must not be nil")
	}

	auditMw, err := newAuditMiddleware(
		opts.Logger, internalgen.GetSwagger, opts.AuditEmitter, opts.AuditEnabled)
	if err != nil {
		return nil, err
	}

	return []internalgen.MiddlewareFunc{
		auditMw.Handler,
		observermiddleware.Recovery(opts.Logger),
		apilogger.Middleware(opts.Logger),
	}, nil
}
