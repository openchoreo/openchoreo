// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	apiaudit "github.com/openchoreo/openchoreo/internal/openchoreo-api/audit"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
	apilogger "github.com/openchoreo/openchoreo/internal/server/middleware/logger"
)

// Handler implements gen.StrictServerInterface
type Handler struct {
	services *handlerservices.Services
	logger   *slog.Logger
	Config   *config.Config
}

// Compile-time check that Handler implements StrictServerInterface
var _ gen.StrictServerInterface = (*Handler)(nil)

// New creates a new Handler
func New(svc *handlerservices.Services, logger *slog.Logger, cfg *config.Config) *Handler {
	return &Handler{
		services: svc,
		logger:   logger,
		Config:   cfg,
	}
}

// InitJWTMiddleware initializes the JWT authentication middleware from the unified configuration.
func InitJWTMiddleware(cfg *config.Config, logger *slog.Logger) func(http.Handler) http.Handler {
	jwtCfg := &cfg.Security.Authentication.JWT

	// Create OAuth2 user type resolver from configuration
	var resolver *jwt.Resolver
	subjectUserTypes := cfg.Security.ToSubjectUserTypeConfigs()
	if len(subjectUserTypes) > 0 {
		var err error
		resolver, err = jwt.NewResolver(subjectUserTypes)
		if err != nil {
			logger.Error("Failed to create OAuth2 user type resolver", "error", err)
			// Continue without resolver - JWT middleware will still work but won't resolve SubjectContext
		}
	}

	return jwt.Middleware(jwtCfg.ToJWTMiddlewareConfig(&cfg.Identity.OIDC, logger, resolver, cfg.Security.Enabled))
}

// APIMiddlewareOptions carries the dependencies APIMiddlewares needs.
type APIMiddlewareOptions struct {
	Logger         *slog.Logger
	AuthMiddleware func(http.Handler) http.Handler // auth.OpenAPIAuth(...) in production
	// AuditEmitter is the single *audit.Emitter shared with the MCP adapter,
	// so one policy applies identically on both surfaces. Must not be nil.
	AuditEmitter *audit.Emitter
	// AuditEnabled mirrors config.AuditConfig.Enabled.
	AuditEnabled bool
}

// APIMiddlewares returns the ordered middleware chain for the generated OpenAPI routes.
//
// oapi-codegen applies these last-to-first, so the last entry is outermost:
//
//	logger → auth → audit → webhookRawBody → handler
//
// audit sits inside auth so SubjectContext is already populated; auditing
// auth failures themselves needs a separate outer instance (P1).
// webhookRawBody stays innermost so HMAC validation sees the raw bytes.
//
// This is the single definition of the chain — main.go supplies dependencies
// but owns no ordering, and TestAuditMiddlewareWired drives exactly this
// function.
//
// Returns an error rather than panicking on a misconfiguration (nil emitter,
// or audit.NewMiddleware failing to build its pattern map) so main can report
// it through the same logger.Error + os.Exit(1) path as every other startup
// failure, instead of an unhandled panic's stack trace.
func APIMiddlewares(opts APIMiddlewareOptions) ([]gen.MiddlewareFunc, error) {
	if opts.AuditEmitter == nil {
		return nil, errors.New("audit: APIMiddlewareOptions.AuditEmitter must not be nil")
	}

	auditMw, err := audit.NewMiddleware(opts.Logger, apiaudit.GetOperations(), gen.GetSwagger, opts.AuditEmitter, opts.AuditEnabled)
	if err != nil {
		return nil, fmt.Errorf("audit: %w", err)
	}

	loggerMw := apilogger.LoggerMiddleware(opts.Logger.With("component", "openapi"))

	return []gen.MiddlewareFunc{
		WebhookRawBodyMiddleware,
		OptionalTriggerBodyMiddleware,
		auditMw.Handler,
		opts.AuthMiddleware,
		loggerMw,
	}, nil
}
