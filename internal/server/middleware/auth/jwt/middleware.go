// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// Middleware creates a new JWT authentication middleware with the given configuration
func Middleware(config Config) func(http.Handler) http.Handler {
	// Set defaults
	config.setDefaults()

	// If middleware is disabled, return a passthrough middleware
	if config.Disabled {
		config.Logger.Warn("JWT authentication middleware is DISABLED - all requests will pass through without authentication")
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			})
		}
	}

	// Validate configuration
	if err := config.validate(); err != nil {
		config.Logger.Error("JWT middleware configuration error", "error", err)
		// Return a middleware that always rejects requests with a generic server error
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				config.Logger.Error("JWT middleware configuration error",
					"error", err,
					"path", r.URL.Path,
					"method", r.Method,
				)
				writeErrorResponse(
					w,
					http.StatusInternalServerError,
					"Server error occurred while authenticating the user",
					"INTERNAL_ERROR",
				)
			})
		}
	}

	// Parse token lookup configuration
	extractor, err := createTokenExtractor(config.TokenLookup)
	if err != nil {
		config.Logger.Error("Invalid TokenLookup configuration", "error", err)
		// Return a middleware that always rejects requests with a generic server error
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				config.Logger.Error("Invalid TokenLookup configuration",
					"error", err,
					"path", r.URL.Path,
					"method", r.Method,
				)
				writeErrorResponse(
					w,
					http.StatusInternalServerError,
					"Server error occurred while authenticating the user",
					"INTERNAL_ERROR",
				)
			})
		}
	}

	// Initialize JWKS cache if JWKS URL is provided
	var cache *jwksCache
	if config.JWKSURL != "" {
		cache = &jwksCache{
			keys:            make(map[string]*cachedJWK),
			jwksURL:         config.JWKSURL,
			refreshInterval: config.JWKSRefreshInterval,
			httpClient:      config.HTTPClient,
			logger:          config.Logger,
		}

		// Start background refresh goroutine
		cache.startBackgroundRefresh()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token from request
			tokenString, err := extractor(r)
			if err != nil {
				writeErrorResponse(w, http.StatusUnauthorized, ErrMissingToken.Error(), CodeMissingToken)
				return
			}

			// Parse and validate token
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				// Extract algorithm from token header
				alg, ok := token.Header["alg"].(string)
				if !ok {
					return nil, errors.New("token missing 'alg' header")
				}

				// Validate algorithm against configured value if specified
				if config.SignatureAlgorithm != "" && alg != config.SignatureAlgorithm {
					return nil, fmt.Errorf(
						"algorithm not allowed: token uses '%s' but only '%s' is accepted",
						alg,
						config.SignatureAlgorithm,
					)
				}

				// Use JWKS if available
				if cache != nil {
					kid, ok := token.Header["kid"].(string)
					if !ok {
						return nil, errors.New("token missing 'kid' header")
					}

					// Refresh cache if needed
					if err := cache.refresh(); err != nil {
						config.Logger.Warn("Failed to refresh JWKS cache", "error", err)
					}

					return cache.getKey(kid, alg)
				}

				// Fall back to static signing key
				return config.SigningKey, nil
			})

			if err != nil {
				config.Logger.Debug("Token validation failed",
					"error", err,
					"path", r.URL.Path,
					"method", r.Method,
				)
				writeErrorResponse(w, http.StatusUnauthorized, ErrInvalidToken.Error(), CodeInvalidToken)
				return
			}

			// Extract claims
			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok || !token.Valid {
				config.Logger.Debug("Invalid token claims",
					"path", r.URL.Path,
					"method", r.Method,
				)
				writeErrorResponse(w, http.StatusUnauthorized, ErrInvalidClaims.Error(), CodeInvalidClaims)
				return
			}

			// Validate custom claims
			if err := validateClaims(claims, config); err != nil {
				config.Logger.Debug("Token claims validation failed",
					"error", err,
					"path", r.URL.Path,
					"method", r.Method,
				)
				writeErrorResponse(w, http.StatusUnauthorized, ErrInvalidClaims.Error(), CodeInvalidClaims)
				return
			}

			// Call success handler if provided
			if config.SuccessHandler != nil {
				if err := config.SuccessHandler(w, r, claims); err != nil {
					config.Logger.Debug("Authorization failed",
						"error", err,
						"path", r.URL.Path,
						"method", r.Method,
					)
					writeErrorResponse(w, http.StatusForbidden, ErrAuthorizationFailed.Error(), CodeAuthorizationFailed)
					return
				}
			}

			// Add claims and token to request context
			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			ctx = context.WithValue(ctx, tokenContextKey, tokenString)

			// Resolve SubjectContext if detector is provided
			if config.Detector != nil {
				subjectCtx, err := config.Detector.ResolveUserType(tokenString)
				if err != nil {
					config.Logger.Debug("Failed to resolve subject context",
						"error", err,
						"path", r.URL.Path,
						"method", r.Method,
					)
					writeErrorResponse(w, http.StatusUnauthorized, "Failed to resolve subject from token", CodeInvalidClaims)
					return
				}

				// Store SubjectContext in context using the auth package helper
				ctx = auth.SetSubjectContext(ctx, subjectCtx)

				config.Logger.Debug("JWT authentication successful with subject resolution",
					"path", r.URL.Path,
					"method", r.Method,
					"subject", claims["sub"],
					"subject_type", subjectCtx.Type,
					"entitlement_claim", subjectCtx.EntitlementClaim,
				)
			} else {
				config.Logger.Debug("JWT authentication successful",
					"path", r.URL.Path,
					"method", r.Method,
					"subject", claims["sub"],
				)
			}

			// Continue with the request
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// validateClaims validates custom claims based on configuration
func validateClaims(claims jwt.MapClaims, config Config) error {
	// Validate issuer
	if config.ValidateIssuer != "" {
		iss, ok := claims["iss"].(string)
		if !ok || iss != config.ValidateIssuer {
			return fmt.Errorf("invalid issuer: expected %s", config.ValidateIssuer)
		}
	}

	// Validate audience only if configured
	if config.ValidateAudience != "" {
		aud, ok := claims["aud"]
		if !ok {
			return errors.New("missing audience claim")
		}

		// Audience can be string or []string
		valid := false
		switch v := aud.(type) {
		case string:
			valid = v == config.ValidateAudience
		case []interface{}:
			for _, a := range v {
				if str, ok := a.(string); ok && str == config.ValidateAudience {
					valid = true
					break
				}
			}
		}

		if !valid {
			return fmt.Errorf("invalid audience: expected %s", config.ValidateAudience)
		}
	}

	return nil
}
