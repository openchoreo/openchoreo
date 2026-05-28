// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package githuboidc validates GitHub Actions OIDC tokens and exposes the
// extracted claims to downstream handlers via the request context.
//
// GitHub Actions workflows can request an OIDC ID token from
// https://token.actions.githubusercontent.com and present it as a Bearer
// token to external APIs. openchoreo-api accepts these tokens (in addition
// to the existing IDP-issued Bearer JWTs) so workflows do not need to hold
// long-lived secrets.
package githuboidc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// DefaultIssuer is the canonical GitHub Actions OIDC issuer.
const DefaultIssuer = "https://token.actions.githubusercontent.com"

// SubjectType is the SubjectContext.Type set on requests authenticated via
// GitHub Actions OIDC. Authorization policies key off this value.
const SubjectType = "github_actions"

// Claims is the subset of the GitHub Actions OIDC token claims that
// downstream handlers consume. All fields are populated directly from the
// signed JWT; nothing else is trusted.
type Claims struct {
	Issuer          string `json:"iss"`
	Subject         string `json:"sub"`
	Audience        string `json:"-"`
	Repository      string `json:"repository"`
	RepositoryID    string `json:"repository_id"`
	RepositoryOwner string `json:"repository_owner"`
	Ref             string `json:"ref"`
	SHA             string `json:"sha"`
	Workflow        string `json:"workflow"`
	WorkflowRef     string `json:"workflow_ref"`
	JobWorkflowRef  string `json:"job_workflow_ref"`
	RunID           string `json:"run_id"`
	RunAttempt      string `json:"run_attempt"`
	Actor           string `json:"actor"`
	EventName       string `json:"event_name"`
}

// Config configures the middleware.
type Config struct {
	// Disabled short-circuits the middleware (the next handler is invoked
	// without any token validation). Used when the GitHub OIDC integration
	// is turned off at the platform level.
	Disabled bool
	// Issuer is the expected `iss` claim. Defaults to DefaultIssuer.
	Issuer string
	// Audience is the expected `aud` claim. MUST be set when Disabled is
	// false; workflows configure their requested audience to match.
	Audience string
	// ClockSkew is the allowed clock skew when validating exp/nbf/iat.
	ClockSkew time.Duration
	// HTTPClient is used for JWKS discovery / refresh. Optional.
	HTTPClient *http.Client
	// Logger receives structured diagnostics.
	Logger *slog.Logger
}

// Verifier wraps go-oidc with the GitHub-specific configuration so the
// middleware itself stays small and easy to test.
type Verifier struct {
	cfg      Config
	verifier *oidc.IDTokenVerifier
	logger   *slog.Logger
}

// NewVerifier discovers the provider and constructs an IDTokenVerifier.
// The discovery happens at construction time so configuration errors
// surface at process startup rather than on the first request.
func NewVerifier(ctx context.Context, cfg Config) (*Verifier, error) {
	if cfg.Audience == "" {
		return nil, errors.New("githuboidc: Audience is required")
	}
	if cfg.Issuer == "" {
		cfg.Issuer = DefaultIssuer
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	clientCtx := ctx
	if cfg.HTTPClient != nil {
		clientCtx = oidc.ClientContext(ctx, cfg.HTTPClient)
	}
	provider, err := oidc.NewProvider(clientCtx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("githuboidc: discover provider %q: %w", cfg.Issuer, err)
	}
	verifier := provider.Verifier(&oidc.Config{
		ClientID:          cfg.Audience,
		SkipClientIDCheck: false,
	})
	return &Verifier{cfg: cfg, verifier: verifier, logger: cfg.Logger}, nil
}

// VerifyToken validates the raw JWT and returns the parsed claims.
func (v *Verifier) VerifyToken(ctx context.Context, raw string) (*Claims, error) {
	idToken, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("verify token: %w", err)
	}
	var claims Claims
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}
	claims.Audience = v.cfg.Audience
	if claims.Repository == "" {
		return nil, errors.New("token missing required `repository` claim")
	}
	return &claims, nil
}

// Middleware returns an HTTP middleware that validates the Bearer token as a
// GitHub Actions OIDC token, stores the parsed claims in the request
// context, and sets a SubjectContext so downstream authorization can route
// off it. Tokens whose `iss` claim does not match the configured issuer are
// rejected immediately so callers can chain a second verifier upstream.
func Middleware(v *Verifier) func(http.Handler) http.Handler {
	if v == nil || v.cfg.Disabled {
		return passthrough()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, err := extractBearer(r)
			if err != nil {
				writeUnauthorized(w, "missing or malformed Authorization header")
				return
			}
			claims, err := v.VerifyToken(r.Context(), raw)
			if err != nil {
				v.logger.Debug("github oidc token rejected",
					"error", err,
					"path", r.URL.Path,
					"method", r.Method,
				)
				writeUnauthorized(w, "invalid GitHub Actions OIDC token")
				return
			}
			ctx := WithClaims(r.Context(), claims)
			ctx = auth.SetSubjectContext(ctx, &auth.SubjectContext{
				ID:                "github_actions:" + claims.Repository,
				Type:              SubjectType,
				EntitlementClaim:  "repository",
				EntitlementValues: []string{claims.Repository},
			})
			v.logger.Debug("github oidc authentication successful",
				"path", r.URL.Path,
				"method", r.Method,
				"repository", claims.Repository,
				"ref", claims.Ref,
				"run_id", claims.RunID,
			)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func passthrough() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
}

func extractBearer(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", errors.New("missing Authorization header")
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", errors.New("malformed Authorization header")
	}
	return parts[1], nil
}

func writeUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="openchoreo-api"`)
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized","message":"` + msg + `"}`))
}
