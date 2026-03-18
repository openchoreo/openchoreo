// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCORS(t *testing.T) {
	// nextCalled tracks whether the wrapped handler was invoked.
	newNext := func(called *bool) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			*called = true
			w.WriteHeader(http.StatusOK)
		})
	}

	tests := []struct {
		name           string
		allowedOrigins []string
		method         string
		headers        map[string]string
		expectNext     bool
		expectStatus   int
		expectHeaders  map[string]string // headers that must be present with exact value
		rejectHeaders  []string          // headers that must NOT be present
	}{
		{
			name:           "empty allowedOrigins passes through without CORS headers",
			allowedOrigins: nil,
			method:         http.MethodGet,
			headers:        map[string]string{"Origin": "http://example.com"},
			expectNext:     true,
			expectStatus:   http.StatusOK,
			rejectHeaders:  []string{"Access-Control-Allow-Origin", "Vary"},
		},
		{
			name:           "allowed origin sets CORS headers and calls next",
			allowedOrigins: []string{"http://localhost:3000"},
			method:         http.MethodGet,
			headers:        map[string]string{"Origin": "http://localhost:3000"},
			expectNext:     true,
			expectStatus:   http.StatusOK,
			expectHeaders: map[string]string{
				"Access-Control-Allow-Origin":  "http://localhost:3000",
				"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS",
				"Access-Control-Allow-Headers": "Content-Type, Authorization",
				"Access-Control-Max-Age":       "3600",
				"Vary":                         "Origin",
			},
		},
		{
			name:           "disallowed origin sets no CORS headers but calls next",
			allowedOrigins: []string{"http://localhost:3000"},
			method:         http.MethodGet,
			headers:        map[string]string{"Origin": "http://evil.com"},
			expectNext:     true,
			expectStatus:   http.StatusOK,
			rejectHeaders:  []string{"Access-Control-Allow-Origin", "Vary"},
		},
		{
			name:           "CORS preflight returns 204 and does not call next",
			allowedOrigins: []string{"http://localhost:3000"},
			method:         http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "http://localhost:3000",
				"Access-Control-Request-Method": "POST",
			},
			expectNext:   false,
			expectStatus: http.StatusNoContent,
			expectHeaders: map[string]string{
				"Access-Control-Allow-Origin": "http://localhost:3000",
			},
		},
		{
			name:           "non-CORS OPTIONS request passes through to next handler",
			allowedOrigins: []string{"http://localhost:3000"},
			method:         http.MethodOptions,
			headers:        nil,
			expectNext:     true,
			expectStatus:   http.StatusOK,
			rejectHeaders:  []string{"Access-Control-Allow-Origin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nextCalled bool
			handler := CORS(tt.allowedOrigins)(newNext(&nextCalled))

			req := httptest.NewRequest(tt.method, "/test", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectNext, nextCalled, "next handler called")
			assert.Equal(t, tt.expectStatus, rec.Code, "status code")

			for header, want := range tt.expectHeaders {
				assert.Equal(t, want, rec.Header().Get(header), "header %q", header)
			}

			for _, header := range tt.rejectHeaders {
				assert.Empty(t, rec.Header().Get(header), "header %q should be absent", header)
			}
		})
	}
}
