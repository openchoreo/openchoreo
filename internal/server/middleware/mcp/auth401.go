// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"net/http"
	"strings"

	"github.com/openchoreo/openchoreo/internal/server/middleware"
)

// responseWriter401Interceptor wraps http.ResponseWriter to intercept 401 status codes
type responseWriter401Interceptor struct {
	http.ResponseWriter
	statusCode      int
	headerWritten   bool
	challengeHeader string
}

// WriteHeader intercepts the status code and adds WWW-Authenticate header on 401
func (rw *responseWriter401Interceptor) WriteHeader(statusCode int) {
	if rw.headerWritten {
		return
	}

	rw.statusCode = statusCode
	rw.headerWritten = true

	// Add WWW-Authenticate header if status is 401
	if statusCode == http.StatusUnauthorized {
		rw.ResponseWriter.Header().Set("WWW-Authenticate", rw.challengeHeader)
	}

	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write intercepts the write to ensure WriteHeader is called
func (rw *responseWriter401Interceptor) Write(b []byte) (int, error) {
	// If WriteHeader hasn't been called yet, call it with 200
	// This handles cases where the handler writes directly without calling WriteHeader
	if !rw.headerWritten {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// Auth401Interceptor creates a middleware that adds WWW-Authenticate header on 401 responses.
// resourceMetadataURL is the URL to the OAuth protected resource metadata endpoint.
// scopes is the space-delimited scope list advertised in the challenge so MCP clients
// pick the resource's required scopes (MCP 2025-06-18 §Authorization scope selection)
// instead of falling back to the authorization server's scopes_supported.
func Auth401Interceptor(resourceMetadataURL string, scopes []string) middleware.Middleware {
	challenge := buildChallengeHeader(resourceMetadataURL, scopes)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wrap the response writer to intercept status codes
			interceptor := &responseWriter401Interceptor{
				ResponseWriter:  w,
				statusCode:      http.StatusOK,
				headerWritten:   false,
				challengeHeader: challenge,
			}

			// Call the next handler with our wrapped response writer
			next.ServeHTTP(interceptor, r)
		})
	}
}

func buildChallengeHeader(resourceMetadataURL string, scopes []string) string {
	var b strings.Builder
	b.WriteString(`Bearer resource_metadata="`)
	b.WriteString(resourceMetadataURL)
	b.WriteString(`"`)
	if scope := strings.TrimSpace(strings.Join(scopes, " ")); scope != "" {
		b.WriteString(`, scope="`)
		b.WriteString(scope)
		b.WriteString(`"`)
	}
	return b.String()
}
