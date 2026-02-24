// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"sort"
	"strings"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// GetOAuthProtectedResourceMetadata returns OAuth 2.0 protected resource metadata
// as defined in RFC 9728, extended with OpenChoreo-specific client configurations.
func (h *Handler) GetOAuthProtectedResourceMetadata(
	ctx context.Context,
	request gen.GetOAuthProtectedResourceMetadataRequestObject,
) (gen.GetOAuthProtectedResourceMetadataResponseObject, error) {
	clients := make([]gen.OpenChoreoClient, 0, len(h.Config.Identity.Clients))
	for name, client := range h.Config.Identity.Clients {
		clients = append(clients, gen.OpenChoreoClient{
			Name:     name,
			ClientId: client.ClientID,
			Scopes:   client.Scopes,
		})
	}
	sort.Slice(clients, func(i, j int) bool {
		return clients[i].Name < clients[j].Name
	})

	securityEnabled := h.Config.Security.Authentication.JWT.Enabled
	resource := strings.TrimSuffix(h.Config.Server.PublicURL, "/") + "/mcp"

	response := gen.GetOAuthProtectedResourceMetadata200JSONResponse{
		ResourceName:         "OpenChoreo MCP Server",
		Resource:             resource,
		AuthorizationServers: []string{h.Config.Identity.OIDC.Issuer},
		BearerMethodsSupported: []string{
			"header",
		},
		ScopesSupported:           []string{},
		OpenchoreoSecurityEnabled: &securityEnabled,
	}

	if len(clients) > 0 {
		response.OpenchoreoClients = &clients
	}

	return response, nil
}
