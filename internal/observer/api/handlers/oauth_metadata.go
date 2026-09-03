// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
)

// OAuthMetadataConfig carries what the protected-resource metadata advertises.
// main.go resolves these from the environment; the handler only renders them.
type OAuthMetadataConfig struct {
	// ResourceName is the human-readable name of the protected resource.
	ResourceName string
	// ResourceURL is the URL the metadata describes, i.e. the observer's /mcp endpoint.
	ResourceURL string
	// AuthorizationServers are the issuers a client may obtain a token from.
	AuthorizationServers []string
	// ScopesSupported is advertised as scopes_supported. MCP clients prefer this
	// over the authorization server's own list, which matters when the AS
	// advertises pool-level scopes a specific app client does not allow.
	ScopesSupported []string
	// SecurityEnabled reports whether authentication is enforced on this server.
	SecurityEnabled bool
}

// GetOAuthProtectedResourceMetadata handles GET /.well-known/oauth-protected-resource
//
// RFC 9728 metadata for the observer's /mcp endpoint. The spec marks it
// `security: []`, which is required rather than incidental: a client has to read
// this to discover where to authenticate, so it cannot itself need a token.
//
// This mirrors openchoreo-api's handler of the same name.
func (h *Handler) GetOAuthProtectedResourceMetadata(
	_ context.Context,
	_ gen.GetOAuthProtectedResourceMetadataRequestObject,
) (gen.GetOAuthProtectedResourceMetadataResponseObject, error) {
	// scopes_supported is required by the schema, so emit [] rather than null
	// when nothing is configured.
	scopes := h.oauthMetadata.ScopesSupported
	if scopes == nil {
		scopes = []string{}
	}

	securityEnabled := h.oauthMetadata.SecurityEnabled

	return jsonResponse(http.StatusOK, gen.OAuthProtectedResourceMetadata{
		ResourceName:              h.oauthMetadata.ResourceName,
		Resource:                  h.oauthMetadata.ResourceURL,
		AuthorizationServers:      h.oauthMetadata.AuthorizationServers,
		BearerMethodsSupported:    []string{"header"},
		ScopesSupported:           scopes,
		OpenchoreoSecurityEnabled: &securityEnabled,
	}), nil
}
