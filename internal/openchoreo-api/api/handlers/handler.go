// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/githuboidc"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
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

// InitGitHubOIDCMiddleware initializes the GitHub Actions OIDC authentication
// middleware. Returns nil (caller treats as disabled) when the integration is
// turned off in configuration. Construction performs an OIDC discovery
// request against the configured issuer, so configuration errors surface at
// process startup rather than on the first authenticated request.
func InitGitHubOIDCMiddleware(
	ctx context.Context,
	cfg *config.Config,
	logger *slog.Logger,
) (func(http.Handler) http.Handler, error) {
	ghaCfg := cfg.Security.Authentication.GitHubOIDC
	if !ghaCfg.Enabled || !cfg.Security.Enabled {
		return nil, nil
	}
	verifier, err := githuboidc.NewVerifier(ctx, githuboidc.Config{
		Issuer:   ghaCfg.Issuer,
		Audience: ghaCfg.Audience,
		Logger:   logger.With("component", "github-oidc"),
	})
	if err != nil {
		return nil, err
	}
	return githuboidc.Middleware(verifier), nil
}
