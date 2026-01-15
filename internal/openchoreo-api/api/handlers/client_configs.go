// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"os"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

// GetOpenIDConfiguration returns OpenID Connect configuration for CLI authentication
func (h *Handler) GetOpenIDConfiguration(
	ctx context.Context,
	request gen.GetOpenIDConfigurationRequestObject,
) (gen.GetOpenIDConfigurationResponseObject, error) {
	// Read endpoints from environment variables
	authorizationEndpoint := os.Getenv(config.EnvOIDCAuthorizationURL)
	tokenEndpoint := os.Getenv(config.EnvOIDCTokenURL)

	// Read new OIDC fields
	issuer := os.Getenv(config.EnvJWTIssuer)
	jwksURI := os.Getenv(config.EnvJWKSURL)
	jwtDisabled := os.Getenv(config.EnvJWTDisabled) == "true"
	securityEnabled := !jwtDisabled

	// Build external clients from config file
	// TODO: This is IdP concern, not resource server. Consider fetching from IdP instead.
	externalClients := make([]gen.ExternalClient, len(h.Config.OAuth.Clients))
	for i, client := range h.Config.OAuth.Clients {
		externalClients[i] = gen.ExternalClient{
			Name:     client.Name,
			ClientId: client.ClientID,
			Scopes:   client.Scopes,
		}
	}

	response := gen.GetOpenIDConfiguration200JSONResponse{
		AuthorizationEndpoint: authorizationEndpoint,
		TokenEndpoint:         tokenEndpoint,
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
