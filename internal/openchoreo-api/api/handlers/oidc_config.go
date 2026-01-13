// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"os"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

// GetOidcConfig returns OIDC configuration
func (h *Handler) GetOidcConfig(
	ctx context.Context,
	request gen.GetOidcConfigRequestObject,
) (gen.GetOidcConfigResponseObject, error) {
	// Read all values from environment variables
	authorizationEndpoint := os.Getenv(config.EnvOIDCAuthorizationURL)
	tokenEndpoint := os.Getenv(config.EnvOIDCTokenURL)

	response := gen.GetOidcConfig200JSONResponse{
		TokenEndpoint: tokenEndpoint,
	}

	if authorizationEndpoint != "" {
		response.AuthorizationEndpoint = &authorizationEndpoint
	}

	return response, nil
}
