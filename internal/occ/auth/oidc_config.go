// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OIDCConfig represents the client configuration response for CLI
type OIDCConfig struct {
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	ClientID              string   `json:"client_id"`
	Scopes                []string `json:"scopes"`
	SecurityEnabled       bool     `json:"security_enabled"`
	Issuer                string   `json:"issuer,omitempty"`
	JwksURI               string   `json:"jwks_uri,omitempty"`
}

// OpenIDConfigurationResponse represents the response from /.well-known/openid-configuration
type OpenIDConfigurationResponse struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	SecurityEnabled       bool   `json:"security_enabled"`
	Issuer                string `json:"issuer,omitempty"`
	JwksURI               string `json:"jwks_uri,omitempty"`
	ExternalClients       []struct {
		Name     string   `json:"name"`
		ClientID string   `json:"client_id"`
		Scopes   []string `json:"scopes"`
	} `json:"external_clients"`
}

// FetchOIDCConfig fetches client configuration for CLI from the OpenID configuration endpoint
func FetchOIDCConfig(apiURL string) (*OIDCConfig, error) {
	req, err := http.NewRequest(
		http.MethodGet,
		apiURL+"/.well-known/openid-configuration",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Use-OpenAPI", "true")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OIDC config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OIDC config request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var configsResp OpenIDConfigurationResponse
	if err := json.NewDecoder(resp.Body).Decode(&configsResp); err != nil {
		return nil, fmt.Errorf("failed to decode OIDC config: %w", err)
	}

	if configsResp.AuthorizationEndpoint == "" {
		return nil, fmt.Errorf("authorization_endpoint not found in OIDC config")
	}
	if configsResp.TokenEndpoint == "" {
		return nil, fmt.Errorf("token_endpoint not found in OIDC config")
	}

	// Find CLI client configuration by name
	var cliClient *OIDCConfig
	for _, client := range configsResp.ExternalClients {
		if client.Name == "cli" {
			cliClient = &OIDCConfig{
				AuthorizationEndpoint: configsResp.AuthorizationEndpoint,
				TokenEndpoint:         configsResp.TokenEndpoint,
				ClientID:              client.ClientID,
				Scopes:                client.Scopes,
				SecurityEnabled:       configsResp.SecurityEnabled,
				Issuer:                configsResp.Issuer,
				JwksURI:               configsResp.JwksURI,
			}
			break
		}
	}

	if cliClient == nil {
		return nil, fmt.Errorf("CLI client configuration (name='cli') not found in external_clients")
	}

	return cliClient, nil
}
