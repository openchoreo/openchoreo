// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package oauth

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ProtectedResourceMetadata represents OAuth 2.0 protected resource metadata
// as defined in RFC 8693 and related OAuth standards
type ProtectedResourceMetadata struct {
	ResourceName           string   `json:"resource_name"`
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ScopesSupported        []string `json:"scopes_supported"`
}

// MetadataHandlerConfig holds configuration for the OAuth metadata handler
type MetadataHandlerConfig struct {
	ResourceName         string
	ResourceURL          string
	AuthorizationServers []string
	Logger               *slog.Logger
}

// NewMetadataHandler creates an HTTP handler that serves OAuth 2.0 protected resource metadata
func NewMetadataHandler(config MetadataHandlerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		metadata := ProtectedResourceMetadata{
			ResourceName:         config.ResourceName,
			Resource:             config.ResourceURL,
			AuthorizationServers: config.AuthorizationServers,
			BearerMethodsSupported: []string{
				"header",
			},
			ScopesSupported: []string{},
		}

		// Encode to ensure no errors before committing response
		data, err := json.Marshal(metadata)
		if err != nil {
			if config.Logger != nil {
				config.Logger.Error("Failed to encode OAuth metadata response", slog.Any("error", err))
			}
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Set response headers and write response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(data); err != nil && config.Logger != nil {
			config.Logger.Error("Failed to write OAuth metadata response", slog.Any("error", err))
		}
	}
}
