// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

// OAuthProtectedResourceMetadata handles requests for OAuth 2.0 protected resource metadata
// as defined in RFC 8693 and related OAuth standards
func (h *Handler) OAuthProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	// Get configuration from environment variables
	serverBaseURL := os.Getenv(config.EnvServerBaseURL)
	if serverBaseURL == "" {
		serverBaseURL = config.DefaultServerBaseURL
	}

	thunderBaseURL := os.Getenv(config.EnvAuthServerBaseURL)
	if thunderBaseURL == "" {
		thunderBaseURL = config.DefaultThunderBaseURL
	}

	// Build metadata response
	metadata := map[string]interface{}{
		"resource_name": "OpenChoreo MCP Server",
		"resource":      serverBaseURL + "/mcp",
		"authorization_servers": []string{
			thunderBaseURL,
		},
		"bearer_methods_supported": []string{
			"header",
		},
		"scopes_supported": []string{},
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Encode and send response
	if err := json.NewEncoder(w).Encode(metadata); err != nil {
		h.logger.Error("Failed to encode OAuth metadata response", slog.Any("error", err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
