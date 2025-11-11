// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// tokenExtractor is a function that extracts a JWT token from an HTTP request
type tokenExtractor func(*http.Request) (string, error)

// createTokenExtractor creates a token extraction function based on the lookup configuration
func createTokenExtractor(lookup string) (tokenExtractor, error) {
	parts := strings.SplitN(lookup, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid token lookup format: expected '<source>:<name>', got '%s'", lookup)
	}

	source := parts[0]
	name := parts[1]

	switch source {
	case "header":
		return extractFromHeader(name), nil
	case "query":
		return extractFromQuery(name), nil
	case "cookie":
		return extractFromCookie(name), nil
	default:
		return nil, fmt.Errorf("invalid token lookup source '%s': must be one of 'header', 'query', or 'cookie'", source)
	}
}

// extractFromHeader creates an extractor that gets the token from an HTTP header
func extractFromHeader(name string) tokenExtractor {
	return func(r *http.Request) (string, error) {
		auth := r.Header.Get(name)
		if auth == "" {
			return "", errors.New("missing authorization header")
		}

		// If using Authorization header, expect Bearer scheme (case-insensitive)
		if name == "Authorization" {
			const bearerPrefix = "bearer "
			authLower := strings.ToLower(auth)
			if !strings.HasPrefix(authLower, bearerPrefix) {
				return "", errors.New("invalid authorization scheme, expected Bearer")
			}
			// Trim the prefix from the original (preserving token case)
			return auth[len(bearerPrefix):], nil
		}

		return auth, nil
	}
}

// extractFromQuery creates an extractor that gets the token from a query parameter
func extractFromQuery(name string) tokenExtractor {
	return func(r *http.Request) (string, error) {
		token := r.URL.Query().Get(name)
		if token == "" {
			return "", errors.New("missing token in query parameter")
		}
		return token, nil
	}
}

// extractFromCookie creates an extractor that gets the token from a cookie
func extractFromCookie(name string) tokenExtractor {
	return func(r *http.Request) (string, error) {
		cookie, err := r.Cookie(name)
		if err != nil {
			return "", fmt.Errorf("missing token cookie: %w", err)
		}
		return cookie.Value, nil
	}
}
