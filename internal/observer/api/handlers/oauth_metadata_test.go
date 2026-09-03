// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// TestGetOAuthProtectedResourceMetadata pins the payload field by field.
//
// MCP clients parse this to find the authorization server and the scopes to ask
// for, so a dropped or renamed field breaks discovery rather than returning an
// error anyone would notice. Field-by-field rather than a golden string, because
// the generated struct orders fields alphabetically and no client should care
// about JSON key order.
func TestGetOAuthProtectedResourceMetadata(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		oauthMetadata: OAuthMetadataConfig{
			ResourceName:         "OpenChoreo Observer MCP Server",
			ResourceURL:          "http://localhost:9097/mcp",
			AuthorizationServers: []string{"https://auth.example.com"},
			ScopesSupported:      []string{"openid", "profile", "email"},
			SecurityEnabled:      false,
		},
	}

	rr := serve(t, h, httptest.NewRequest(
		http.MethodGet, "/.well-known/oauth-protected-resource", nil))

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	// Decode into a map rather than the generated type, so a field silently
	// disappearing from the schema fails here too.
	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))

	assert.Equal(t, "OpenChoreo Observer MCP Server", body["resource_name"])
	assert.Equal(t, "http://localhost:9097/mcp", body["resource"])
	assert.Equal(t, []any{"https://auth.example.com"}, body["authorization_servers"])
	assert.Equal(t, []any{"header"}, body["bearer_methods_supported"])
	assert.Equal(t, []any{"openid", "profile", "email"}, body["scopes_supported"])
	assert.Equal(t, false, body["openchoreo_security_enabled"])

	// Never populated by the observer, so it must stay absent rather than appear
	// as null.
	assert.NotContains(t, body, "openchoreo_clients")
}

// TestGetOAuthProtectedResourceMetadata_NoScopesConfigured checks scopes_supported
// is an empty array rather than null. It is required by the schema, and a null
// there breaks clients that iterate it without a nil check.
func TestGetOAuthProtectedResourceMetadata_NoScopesConfigured(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		oauthMetadata: OAuthMetadataConfig{
			ResourceName: "OpenChoreo Observer MCP Server",
			ResourceURL:  "http://localhost:9097/mcp",
		},
	}

	rr := serve(t, h, httptest.NewRequest(
		http.MethodGet, "/.well-known/oauth-protected-resource", nil))

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"scopes_supported":[]`)
}

// TestGetOAuthProtectedResourceMetadata_NeedsNoToken pins the guarantee: the
// spec marks this operation `security: []`, and a client has to read it before
// it can obtain a token, so requiring one would be circular.
//
// It drives the real auth.OpenAPIAuth, so removing that override fails here.
func TestGetOAuthProtectedResourceMetadata_NeedsNoToken(t *testing.T) {
	t.Parallel()

	rejectAll := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
	}

	h := &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		oauthMetadata: OAuthMetadataConfig{
			ResourceName: "OpenChoreo Observer MCP Server",
			ResourceURL:  "http://localhost:9097/mcp",
		},
	}

	// Wrapped in OpenAPIAuth, as production does: rejectAll then only sees
	// requests the generated wrapper marked as needing a token.
	rr := httptest.NewRecorder()
	newPublicServerWithAuth(t, h, auth.OpenAPIAuth(rejectAll, gen.BearerAuthScopes)).
		ServeHTTP(rr, httptest.NewRequest(
			http.MethodGet, "/.well-known/oauth-protected-resource", nil))

	assert.Equal(t, http.StatusOK, rr.Code,
		"metadata discovery must not require a token")
}
