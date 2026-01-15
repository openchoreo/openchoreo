// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
)

// IsTokenExpired checks if the JWT token is expired or will expire soon (within 1 minute)
func IsTokenExpired(token string) bool {
	if token == "" {
		return false
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return true
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return true
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return true
	}

	// Check if token is expired or will expire within 1 minute
	expiryTime := time.Unix(claims.Exp, 0)
	return time.Now().Add(1 * time.Minute).After(expiryTime)
}

// RefreshToken refreshes the access token using the appropriate auth method
// Returns the new token and an error if refresh fails
func RefreshToken(token string) (string, error) {
	// Get current credential
	credential, err := config.GetCurrentCredential()
	if err != nil {
		return "", err
	}

	// Get current control plane
	controlPlane, err := config.GetCurrentControlPlane()
	if err != nil {
		return "", err
	}

	// Load config for saving updated tokens
	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	// Check auth method and use appropriate refresh strategy
	if credential.AuthMethod == "pkce" && credential.RefreshToken != "" {
		// Use PKCE refresh token grant
		oidcConfig, err := FetchOIDCConfig(controlPlane.URL)
		if err != nil {
			return "", fmt.Errorf("failed to fetch OIDC config: %w", err)
		}

		tokenResp, err := RefreshAccessToken(
			oidcConfig.TokenEndpoint,
			credential.ClientID,
			credential.RefreshToken,
		)
		if err != nil {
			return "", fmt.Errorf("failed to refresh PKCE token: %w", err)
		}

		// Update token in config
		credential.Token = tokenResp.AccessToken
		if tokenResp.RefreshToken != "" {
			credential.RefreshToken = tokenResp.RefreshToken
		}

		if err := config.SaveStoredConfig(cfg); err != nil {
			return "", fmt.Errorf("failed to save updated token: %w", err)
		}

		return tokenResp.AccessToken, nil
	}

	// Fall back to client credentials refresh
	if credential.ClientID == "" || credential.ClientSecret == "" {
		return "", fmt.Errorf("credential does not have client credentials for refresh")
	}

	// Fetch OIDC config from API
	oidcConfig, err := FetchOIDCConfig(controlPlane.URL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch OIDC config: %w", err)
	}

	// Request new token
	authClient := &ClientCredentialsAuth{
		TokenEndpoint: oidcConfig.TokenEndpoint,
		ClientID:      credential.ClientID,
		ClientSecret:  credential.ClientSecret,
	}

	tokenResp, err := authClient.GetToken()
	if err != nil {
		return "", fmt.Errorf("failed to get new access token: %w", err)
	}

	// Update token in config
	credential.Token = tokenResp.AccessToken
	if err := config.SaveStoredConfig(cfg); err != nil {
		return "", fmt.Errorf("failed to save updated token: %w", err)
	}

	return tokenResp.AccessToken, nil
}
