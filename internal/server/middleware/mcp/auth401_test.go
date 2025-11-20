// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuth401Interceptor(t *testing.T) {
	resourceMetadataURL := "http://api.openchoreo.localhost/.well-known/oauth-protected-resource"
	expectedHeader := "Bearer resource_metadata=\"http://api.openchoreo.localhost/.well-known/oauth-protected-resource\""

	tests := []struct {
		name             string
		statusCode       int
		shouldHaveHeader bool
	}{
		{
			name:             "401 response should add WWW-Authenticate header",
			statusCode:       http.StatusUnauthorized,
			shouldHaveHeader: true,
		},
		{
			name:             "200 response should not add WWW-Authenticate header",
			statusCode:       http.StatusOK,
			shouldHaveHeader: false,
		},
		{
			name:             "403 response should not add WWW-Authenticate header",
			statusCode:       http.StatusForbidden,
			shouldHaveHeader: false,
		},
		{
			name:             "500 response should not add WWW-Authenticate header",
			statusCode:       http.StatusInternalServerError,
			shouldHaveHeader: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test handler that returns the specified status code
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})

			// Wrap the handler with the middleware
			middleware := Auth401Interceptor(resourceMetadataURL)
			wrappedHandler := middleware(testHandler)

			// Create a test request and response recorder
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			// Execute the handler
			wrappedHandler.ServeHTTP(rec, req)

			// Check the status code
			if rec.Code != tt.statusCode {
				t.Errorf("expected status code %d, got %d", tt.statusCode, rec.Code)
			}

			// Check for WWW-Authenticate header
			wwwAuth := rec.Header().Get("WWW-Authenticate")
			if tt.shouldHaveHeader {
				if wwwAuth == "" {
					t.Error("expected WWW-Authenticate header to be present, but it was not")
				}
				if wwwAuth != expectedHeader {
					t.Errorf("expected WWW-Authenticate header to be %q, got %q", expectedHeader, wwwAuth)
				}
			} else {
				if wwwAuth != "" {
					t.Errorf("expected no WWW-Authenticate header, but got %q", wwwAuth)
				}
			}
		})
	}
}

func TestAuth401Interceptor_WithWrite(t *testing.T) {
	resourceMetadataURL := "http://api.openchoreo.localhost/.well-known/oauth-protected-resource"
	expectedHeader := "Bearer resource_metadata=\"http://api.openchoreo.localhost/.well-known/oauth-protected-resource\""

	// Test handler that writes without explicitly calling WriteHeader
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This should trigger an implicit 200 OK
		_, _ = w.Write([]byte("OK"))
	})

	middleware := Auth401Interceptor(resourceMetadataURL)
	wrappedHandler := middleware(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	// Should have 200 status and no WWW-Authenticate header
	if rec.Code != http.StatusOK {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}

	wwwAuth := rec.Header().Get("WWW-Authenticate")
	if wwwAuth != "" {
		t.Errorf("expected no WWW-Authenticate header for 200 OK, but got %q", wwwAuth)
	}

	// Test handler that writes with 401
	testHandler401 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("Unauthorized"))
	})

	wrappedHandler401 := middleware(testHandler401)

	req401 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec401 := httptest.NewRecorder()

	wrappedHandler401.ServeHTTP(rec401, req401)

	// Should have 401 status and WWW-Authenticate header
	if rec401.Code != http.StatusUnauthorized {
		t.Errorf("expected status code 401, got %d", rec401.Code)
	}

	wwwAuth401 := rec401.Header().Get("WWW-Authenticate")
	if wwwAuth401 == "" {
		t.Error("expected WWW-Authenticate header to be present for 401, but it was not")
	}
	if wwwAuth401 != expectedHeader {
		t.Errorf("expected WWW-Authenticate header to be %q, got %q", expectedHeader, wwwAuth401)
	}
}
