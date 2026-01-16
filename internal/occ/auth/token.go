// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
)

// IsTokenExpired checks if the JWT token is expired or will expire soon (within 1 minute)
func IsTokenExpired(token string) bool {
	if token == "" {
		return false
	}

	// Parse token without validation (we only need to check expiry)
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	claims := jwt.MapClaims{}
	_, _, err := parser.ParseUnverified(token, claims)
	if err != nil {
		return true
	}

	// Get expiry time from claims
	exp, ok := claims["exp"]
	if !ok {
		return true
	}

	var expiryTime time.Time
	switch v := exp.(type) {
	case float64:
		expiryTime = time.Unix(int64(v), 0)
	case int64:
		expiryTime = time.Unix(v, 0)
	default:
		return true
	}

	// Check if token is expired or will expire within 1 minute
	return time.Now().Add(1 * time.Minute).After(expiryTime)
}

// RefreshToken refreshes the access token using the appropriate auth method
// Returns the new token and an error if refresh fails
func RefreshToken() (string, error) {
	credential, err := config.GetCurrentCredential()
	if err != nil {
		return "", err
	}

	controlPlane, err := config.GetCurrentControlPlane()
	if err != nil {
		return "", err
	}

	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	oidcConfig, err := FetchOIDCConfig(controlPlane.URL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch OIDC config: %w", err)
	}

	// Check auth method and use appropriate refresh strategy
	if credential.AuthMethod == "authorization_code" && credential.RefreshToken != "" {
		// Use PKCE refresh token grant
		tokenResp, err := RefreshAccessToken(
			oidcConfig.TokenEndpoint,
			credential.ClientID,
			credential.RefreshToken,
		)
		if err != nil {
			return "", fmt.Errorf("failed to refresh PKCE token: %w", err)
		}

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

	authClient := &ClientCredentialsAuth{
		TokenEndpoint: oidcConfig.TokenEndpoint,
		ClientID:      credential.ClientID,
		ClientSecret:  credential.ClientSecret,
	}

	tokenResp, err := authClient.GetToken()
	if err != nil {
		return "", fmt.Errorf("failed to get new access token: %w", err)
	}

	credential.Token = tokenResp.AccessToken
	if err := config.SaveStoredConfig(cfg); err != nil {
		return "", fmt.Errorf("failed to save updated token: %w", err)
	}

	return tokenResp.AccessToken, nil
}
