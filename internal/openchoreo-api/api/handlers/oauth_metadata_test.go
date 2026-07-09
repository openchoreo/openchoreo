// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

func TestGetOAuthProtectedResourceMetadata(t *testing.T) {
	tests := []struct {
		name       string
		cfgScopes  []string
		wantScopes []string
	}{
		{
			name:       "scopes from config are advertised",
			cfgScopes:  []string{"openid", "profile", "email"},
			wantScopes: []string{"openid", "profile", "email"},
		},
		{
			name:       "nil config scopes yields empty list, never null",
			cfgScopes:  nil,
			wantScopes: []string{},
		},
		{
			name:       "custom scope set is preserved in order",
			cfgScopes:  []string{"api.read", "api.write"},
			wantScopes: []string{"api.read", "api.write"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{
				Config: &config.Config{
					Server: config.ServerConfig{
						PublicURL: "http://api.openchoreo.localhost",
					},
					Identity: config.IdentityConfig{
						OIDC: config.OIDCConfig{
							Issuer: "http://sts.openchoreo.localhost",
						},
						MCPOAuthScopes: tt.cfgScopes,
					},
				},
			}

			resp, err := h.GetOAuthProtectedResourceMetadata(
				context.Background(),
				gen.GetOAuthProtectedResourceMetadataRequestObject{},
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			metadata, ok := resp.(gen.GetOAuthProtectedResourceMetadata200JSONResponse)
			if !ok {
				t.Fatalf("unexpected response type: %T", resp)
			}

			if metadata.Resource != "http://api.openchoreo.localhost/mcp" {
				t.Errorf("Resource: got %q, want %q", metadata.Resource, "http://api.openchoreo.localhost/mcp")
			}

			if len(metadata.AuthorizationServers) != 1 || metadata.AuthorizationServers[0] != "http://sts.openchoreo.localhost" {
				t.Errorf("AuthorizationServers: got %v, want [\"http://sts.openchoreo.localhost\"]", metadata.AuthorizationServers)
			}

			if len(metadata.ScopesSupported) != len(tt.wantScopes) {
				t.Fatalf("ScopesSupported length: got %d, want %d", len(metadata.ScopesSupported), len(tt.wantScopes))
			}
			for i, scope := range tt.wantScopes {
				if metadata.ScopesSupported[i] != scope {
					t.Errorf("ScopesSupported[%d]: got %q, want %q", i, metadata.ScopesSupported[i], scope)
				}
			}

			// The OpenAPI contract marks scopes_supported as required, so the
			// field must serialize as [] rather than null when no scopes are
			// configured. Asserting non-nil protects that JSON encoding.
			if metadata.ScopesSupported == nil {
				t.Error("ScopesSupported must not be nil (required field in RFC 9728 metadata)")
			}
		})
	}
}
