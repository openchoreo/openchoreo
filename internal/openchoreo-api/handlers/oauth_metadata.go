// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"
	"os"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/server/oauth"
)

// OAuthProtectedResourceMetadata handles requests for OAuth 2.0 protected resource metadata
// as defined in RFC 9728 and related OAuth standards
func (h *Handler) OAuthProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	// Get configuration from environment variables
	serverBaseURL := os.Getenv(config.EnvServerBaseURL)
	if serverBaseURL == "" {
		serverBaseURL = config.DefaultServerBaseURL
	}

	authServerBaseURL := os.Getenv(config.EnvAuthServerBaseURL)
	if authServerBaseURL == "" {
		authServerBaseURL = config.DefaultThunderBaseURL
	}

	// Create metadata handler and serve the request
	handler := oauth.NewMetadataHandler(oauth.MetadataHandlerConfig{
		ResourceName: "OpenChoreo MCP Server",
		ResourceURL:  serverBaseURL + "/mcp",
		AuthorizationServers: []string{
			authServerBaseURL,
		},
		Logger: h.logger,
	})

	handler(w, r)
}
