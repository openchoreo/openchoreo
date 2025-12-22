// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Config holds the configuration for JWT authentication middleware
type Config struct {
	// Disabled disables JWT authentication when set to true
	// When disabled, the middleware will pass through all requests without authentication
	// This is useful for local development or testing environments
	// Default: false
	Disabled bool

	// JWKSURL is the URL to fetch the JSON Web Key Set for token validation
	// This is the primary method for key management in production environments
	JWKSURL string

	// JWKSRefreshInterval defines how often to refresh the JWKS from the URL
	// Default: 1 hour
	JWKSRefreshInterval time.Duration

	// SigningKey is an alternative to JWKS for simpler scenarios
	// For HMAC algorithms (HS256, HS384, HS512), this should be a []byte
	// For RSA algorithms (RS256, RS384, RS512), this should be a *rsa.PublicKey
	// Note: If JWKSURL is provided, this field is ignored
	SigningKey interface{}

	// TokenLookup defines where to extract the JWT token from the request
	// Format: "<source>:<name>"
	// Possible values:
	// - "header:<name>" - extract from HTTP header (e.g., "header:Authorization")
	//   When using "header:Authorization", the Bearer scheme is automatically handled
	// - "query:<name>" - extract from query parameter (e.g., "query:token")
	// - "cookie:<name>" - extract from cookie (e.g., "cookie:jwt")
	// Default: "header:Authorization"
	TokenLookup string

	// SuccessHandler is an optional handler called after successful token validation
	// Can be used for additional validation, logging, etc.
	SuccessHandler func(w http.ResponseWriter, r *http.Request, claims jwt.MapClaims) error

	// Logger is an optional slog logger for logging authentication events
	Logger *slog.Logger

	// ValidateIssuer enables issuer validation
	// If set, the token's "iss" claim must match this value
	ValidateIssuer string

	// ValidateAudience enables audience validation (optional)
	// If set, the token's "aud" claim must contain this value
	// If empty, audience validation is skipped
	ValidateAudience string

	// SignatureAlgorithm specifies the expected signature algorithm (optional)
	// Common values: RS256, RS384, RS512, HS256, HS384, HS512, ES256, ES384, ES512
	// If set, incoming tokens must use this algorithm
	// If empty, algorithm validation is skipped (except JWK alg validation if present)
	SignatureAlgorithm string

	// ClockSkew allows for clock skew when validating time-based claims
	// Default: 0 (no skew tolerance)
	ClockSkew time.Duration

	// HTTPClient is the HTTP client used to fetch JWKS
	// If not set, http.DefaultClient is used
	HTTPClient *http.Client

	// JWKSURLTLSInsecureSkipVerify controls whether the HTTP client verifies the server's certificate chain and host name when fetching JWKS
	// If true, TLS accepts any certificate presented by the JWKS server and any host name in that certificate
	// This should only be used for testing or in trusted environments
	// Default: false
	JWKSURLTLSInsecureSkipVerify bool

	// Detector is used to detect user type and extract SubjectContext from JWT tokens
	// If provided, the middleware will resolve SubjectContext and store it in the request context
	// If not provided, only token and claims will be stored in the context
	Detector *Resolver
}

// setDefaults sets default values for unspecified config fields
func (c *Config) setDefaults() {
	if c.TokenLookup == "" {
		c.TokenLookup = "header:Authorization"
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.HTTPClient == nil {
		// Create HTTP client with TLS configuration
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				//nolint:gosec // G402: InsecureSkipVerify is configurable via environment variables for testing purposes. Defaults to false.
				InsecureSkipVerify: c.JWKSURLTLSInsecureSkipVerify,
			},
		}
		c.HTTPClient = &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		}
	}
	if c.JWKSRefreshInterval == 0 {
		c.JWKSRefreshInterval = 1 * time.Hour
	}
}

// validate checks if the configuration is valid
func (c *Config) validate() error {
	// Skip validation if middleware is disabled
	if c.Disabled {
		return nil
	}

	// Either JWKS URL or signing key must be provided
	if c.JWKSURL == "" && c.SigningKey == nil {
		return fmt.Errorf("configuration error: either JWKSURL or SigningKey must be provided")
	}

	return nil
}
