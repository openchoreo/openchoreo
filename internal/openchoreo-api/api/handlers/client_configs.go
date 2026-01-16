// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"os"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

// GetClientConfigs returns client configurations
func (h *Handler) GetClientConfigs(
	ctx context.Context,
	request gen.GetClientConfigsRequestObject,
) (gen.GetClientConfigsResponseObject, error) {
	// Read endpoints from environment variables
	authorizationEndpoint := os.Getenv(config.EnvOIDCAuthorizationURL)
	tokenEndpoint := os.Getenv(config.EnvOIDCTokenURL)

	// Build external clients from config file
	externalClients := make([]gen.ExternalClient, len(h.Config.Security.ExternalClients))
	for i, client := range h.Config.Security.ExternalClients {
		externalClients[i] = gen.ExternalClient{
			Name:     client.Name,
			ClientId: client.ClientID,
			Scopes:   client.Scopes,
		}
	}

	response := gen.GetClientConfigs200JSONResponse{
		AuthorizationEndpoint: authorizationEndpoint,
		TokenEndpoint:         tokenEndpoint,
		ExternalClients:       externalClients,
	}

	return response, nil
}
