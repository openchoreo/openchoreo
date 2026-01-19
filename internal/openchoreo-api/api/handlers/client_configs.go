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
	// Build external clients from config
	externalClients := make([]gen.ExternalClient, len(h.Config.OAuth.Clients))
	for i, client := range h.Config.OAuth.Clients {
		externalClients[i] = gen.ExternalClient{
			Name:     client.Name,
			ClientId: client.ClientID,
			Scopes:   client.Scopes,
		}
	}

	// Get OIDC endpoints and security settings from config
	issuer := h.Config.OAuth.OIDC.Issuer
	jwksURI := h.Config.Middleware.JWT.JWKSURL
	securityEnabled := !h.Config.Middleware.JWT.Disabled

	response := gen.GetOpenIDConfiguration200JSONResponse{
		AuthorizationEndpoint: h.Config.OAuth.OIDC.AuthorizationURL,
		TokenEndpoint:         h.Config.OAuth.OIDC.TokenURL,
		ExternalClients:       externalClients,
		SecurityEnabled:       securityEnabled,
	}

	// Set optional fields only if they have values
	if issuer != "" {
		response.Issuer = &issuer
	}
	if jwksURI != "" {
		response.JwksUri = &jwksURI
	}

	return response, nil
}
