// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"
	"sort"

	"github.com/openchoreo/openchoreo/internal/server/oauth"
)

// OAuthProtectedResourceMetadata handles requests for OAuth 2.0 protected resource metadata
// as defined in RFC 9728 and related OAuth standards
func (h *Handler) OAuthProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	clients := make([]oauth.ClientInfo, 0, len(h.config.Identity.Clients))
	for name, client := range h.config.Identity.Clients {
		clients = append(clients, oauth.ClientInfo{
			Name:     name,
			ClientID: client.ClientID,
			Scopes:   client.Scopes,
		})
	}
	sort.Slice(clients, func(i, j int) bool {
		return clients[i].Name < clients[j].Name
	})

	handler := oauth.NewMetadataHandler(oauth.MetadataHandlerConfig{
		ResourceName: "OpenChoreo MCP Server",
		ResourceURL:  h.config.Server.PublicURL + "/mcp",
		AuthorizationServers: []string{
			h.config.Identity.OIDC.Issuer,
		},
		Clients:         clients,
		SecurityEnabled: h.config.Security.Authentication.JWT.Enabled,
		Logger:          h.logger,
	})

	handler(w, r)
}
