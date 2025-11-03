// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v5"

	"github.com/openchoreo/openchoreo/pkg/middleware/auth"
)

// Middleware creates a new JWT authentication middleware with the given configuration
func Middleware(config Config) func(http.Handler) http.Handler {
	// Set defaults
	config.setDefaults()

	// Validate configuration
	if err := config.validate(); err != nil {
		config.Logger.Error("JWT middleware configuration error", "error", err)
		// Return a middleware that always rejects requests with a generic server error
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				config.ErrorHandler(w, r, auth.NewAuthError(
					auth.CodeInternalError,
					"Server error occurred while authenticating the user",
					http.StatusInternalServerError,
					err,
				))
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
				config.ErrorHandler(w, r, auth.NewAuthError(
					auth.CodeInternalError,
					"Server error occurred while authenticating the user",
					http.StatusInternalServerError,
					err,
				))
			})
		}
	}

	// Initialize JWKS cache if JWKS URL is provided
	var cache *jwksCache
	if config.JWKSURL != "" {
		cache = &jwksCache{
			keys:            make(map[string]*rsa.PublicKey),
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
				config.ErrorHandler(w, r, auth.NewAuthError(
					CodeMissingToken,
					ErrMissingToken.Error(),
					http.StatusUnauthorized,
					err,
				))
				return
			}

			// Parse and validate token
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
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

					return cache.getKey(kid)
				}

				// Fall back to static signing key
				return config.SigningKey, nil
			})

			if err != nil {
				config.ErrorHandler(w, r, auth.NewAuthError(
					CodeInvalidToken,
					ErrInvalidToken.Error(),
					http.StatusUnauthorized,
					err,
				))
				return
			}

			// Extract claims
			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok || !token.Valid {
				config.ErrorHandler(w, r, auth.NewAuthError(
					CodeInvalidClaims,
					ErrInvalidClaims.Error(),
					http.StatusUnauthorized,
					errors.New("invalid claims format"),
				))
				return
			}

			// Validate custom claims
			if err := validateClaims(claims, config); err != nil {
				config.ErrorHandler(w, r, auth.NewAuthError(
					CodeInvalidClaims,
					ErrInvalidClaims.Error(),
					http.StatusUnauthorized,
					err,
				))
				return
			}

			// Call success handler if provided
			if config.SuccessHandler != nil {
				if err := config.SuccessHandler(w, r, claims); err != nil {
					config.ErrorHandler(w, r, auth.NewAuthError(
						CodeAuthorizationFailed,
						ErrAuthorizationFailed.Error(),
						http.StatusForbidden,
						err,
					))
					return
				}
			}

			// Add claims and token to request context
			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			ctx = context.WithValue(ctx, tokenContextKey, tokenString)

			config.Logger.Debug("JWT authentication successful",
				"path", r.URL.Path,
				"method", r.Method,
				"subject", claims["sub"],
			)

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
