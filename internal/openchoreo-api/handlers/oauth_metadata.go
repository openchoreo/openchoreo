// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"

	"github.com/openchoreo/openchoreo/internal/server/oauth"
)

// OAuthProtectedResourceMetadata handles requests for OAuth 2.0 protected resource metadata
// as defined in RFC 9728 and related OAuth standards
func (h *Handler) OAuthProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	// Create metadata handler using unified configuration
	handler := oauth.NewMetadataHandler(oauth.MetadataHandlerConfig{
		ResourceName: "OpenChoreo MCP Server",
		ResourceURL:  h.config.Server.PublicURL + "/mcp",
		AuthorizationServers: []string{
			h.config.Identity.OIDC.Issuer,
		},
		Logger: h.logger,
	})

	handler(w, r)
}
