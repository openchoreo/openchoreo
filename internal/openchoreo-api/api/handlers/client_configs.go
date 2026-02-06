// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// GetOpenIDConfiguration returns OpenID Connect configuration for CLI authentication
func (h *Handler) GetOpenIDConfiguration(
	ctx context.Context,
	request gen.GetOpenIDConfigurationRequestObject,
) (gen.GetOpenIDConfigurationResponseObject, error) {
	// Build external clients from config (Identity.Clients is a map)
	externalClients := make([]gen.ExternalClient, 0, len(h.Config.Identity.Clients))
	for name, client := range h.Config.Identity.Clients {
		externalClients = append(externalClients, gen.ExternalClient{
			Name:     name,
			ClientId: client.ClientID,
			Scopes:   client.Scopes,
		})
	}

	// Get OIDC endpoints and security settings from config
	issuer := h.Config.Identity.OIDC.Issuer
	securityEnabled := h.Config.Security.Authentication.JWT.Enabled

	response := gen.GetOpenIDConfiguration200JSONResponse{
		AuthorizationEndpoint: h.Config.Identity.OIDC.AuthorizationEndpoint,
		TokenEndpoint:         h.Config.Identity.OIDC.TokenEndpoint,
		ExternalClients:       externalClients,
		SecurityEnabled:       securityEnabled,
	}

	// Set optional fields only if they have values
	if issuer != "" {
		response.Issuer = &issuer
	}

	return response, nil
}
