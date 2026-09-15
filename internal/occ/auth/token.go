// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

// refreshMu serializes refreshes within this process; lockTokenFile extends that across
// processes, so no two callers replay a rotated refresh token or write the config at once.
var refreshMu sync.Mutex

// lockTokenFile blocks until it holds the cross-process token lock, returning the
// release.
func lockTokenFile() (func(), error) {
	path, err := config.TokenLockPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open token lock: %w", err)
	}
	if err := lockFD(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("failed to take token lock: %w", err)
	}
	return func() { _ = f.Close() }, nil
}

// CanRefresh reports whether cred carries material RefreshCredential can redeem.
func CanRefresh(cred *config.Credential) bool {
	if cred == nil {
		return false
	}
	if cred.AuthMethod == "authorization_code" && cred.RefreshToken != "" {
		return true
	}
	return cred.ClientID != "" && cred.ClientSecret != ""
}

// RefreshToken refreshes the access token using the appropriate auth method
// Returns the new token and an error if refresh fails
func RefreshToken() (string, error) {
	// One snapshot, so the credential and control plane cannot come from different
	// contexts if the active one changes underneath.
	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}
	controlPlane, err := cfg.ActiveControlPlane()
	if err != nil {
		return "", err
	}
	credential, err := cfg.ActiveCredential()
	if err != nil {
		return "", err
	}
	return RefreshCredential(credential.Name, controlPlane.URL)
}

// RefreshCredential refreshes the named credential against the given control plane,
// independently of which context is selected now. A long-running command pins both at
// start-up so a context switch elsewhere cannot repoint its token mid-session.
func RefreshCredential(credentialName, controlPlaneURL string) (string, error) {
	refreshMu.Lock()
	defer refreshMu.Unlock()

	unlock, err := lockTokenFile()
	if err != nil {
		return "", err
	}
	defer unlock()

	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	// credential aliases cfg, so updating it and saving cfg persists the new tokens.
	credential, err := cfg.CredentialByName(credentialName)
	if err != nil {
		return "", err
	}

	// Another caller may have refreshed while this one waited for the lock.
	if credential.Token != "" && !IsTokenExpired(credential.Token) {
		return credential.Token, nil
	}

	oidcConfig, err := FetchOIDCConfig(controlPlaneURL)
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
			return "", fmt.Errorf("failed to refresh token, please run 'occ login' to re-authenticate: %w", err)
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
		Scope:         credential.Scope,
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
