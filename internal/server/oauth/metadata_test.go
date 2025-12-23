// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewMetadataHandler(t *testing.T) {
	tests := []struct {
		name   string
		config MetadataHandlerConfig
		want   ProtectedResourceMetadata
	}{
		{
			name: "openchoreo-api metadata",
			config: MetadataHandlerConfig{
				ResourceName: "OpenChoreo MCP Server",
				ResourceURL:  "http://api.openchoreo.localhost/mcp",
				AuthorizationServers: []string{
					"http://sts.openchoreo.localhost",
				},
			},
			want: ProtectedResourceMetadata{
				ResourceName: "OpenChoreo MCP Server",
				Resource:     "http://api.openchoreo.localhost/mcp",
				AuthorizationServers: []string{
					"http://sts.openchoreo.localhost",
				},
				BearerMethodsSupported: []string{"header"},
				ScopesSupported:        []string{},
			},
		},
		{
			name: "observer metadata",
			config: MetadataHandlerConfig{
				ResourceName: "OpenChoreo Observer MCP Server",
				ResourceURL:  "http://localhost:9097/mcp",
				AuthorizationServers: []string{
					"http://sts.openchoreo.localhost",
				},
			},
			want: ProtectedResourceMetadata{
				ResourceName: "OpenChoreo Observer MCP Server",
				Resource:     "http://localhost:9097/mcp",
				AuthorizationServers: []string{
					"http://sts.openchoreo.localhost",
				},
				BearerMethodsSupported: []string{"header"},
				ScopesSupported:        []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewMetadataHandler(tt.config)

			req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
			rec := httptest.NewRecorder()

			handler(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("expected status code %d, got %d", http.StatusOK, rec.Code)
			}

			contentType := rec.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("expected Content-Type application/json, got %s", contentType)
			}

			var got ProtectedResourceMetadata
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if got.ResourceName != tt.want.ResourceName {
				t.Errorf("ResourceName: got %q, want %q", got.ResourceName, tt.want.ResourceName)
			}

			if got.Resource != tt.want.Resource {
				t.Errorf("Resource: got %q, want %q", got.Resource, tt.want.Resource)
			}

			if len(got.AuthorizationServers) != len(tt.want.AuthorizationServers) {
				t.Errorf("AuthorizationServers length: got %d, want %d", len(got.AuthorizationServers), len(tt.want.AuthorizationServers))
			}

			for i, server := range got.AuthorizationServers {
				if server != tt.want.AuthorizationServers[i] {
					t.Errorf("AuthorizationServers[%d]: got %q, want %q", i, server, tt.want.AuthorizationServers[i])
				}
			}

			if len(got.BearerMethodsSupported) != len(tt.want.BearerMethodsSupported) {
				t.Errorf("BearerMethodsSupported length: got %d, want %d", len(got.BearerMethodsSupported), len(tt.want.BearerMethodsSupported))
			}

			for i, method := range got.BearerMethodsSupported {
				if method != tt.want.BearerMethodsSupported[i] {
					t.Errorf("BearerMethodsSupported[%d]: got %q, want %q", i, method, tt.want.BearerMethodsSupported[i])
				}
			}
		})
	}
}
