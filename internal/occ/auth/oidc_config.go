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
}

// ClientConfigsResponse represents the response from /api/v1/client-configs
type ClientConfigsResponse struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	ExternalClients       []struct {
		Name     string   `json:"name"`
		ClientID string   `json:"client_id"`
		Scopes   []string `json:"scopes"`
	} `json:"external_clients"`
}

// FetchOIDCConfig fetches client configuration for CLI from the new endpoint
func FetchOIDCConfig(apiURL string) (*OIDCConfig, error) {
	req, err := http.NewRequest(
		http.MethodGet,
		apiURL+"/api/v1/client-configs",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Use-OpenAPI", "true")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch client configs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("client configs request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var configsResp ClientConfigsResponse
	if err := json.NewDecoder(resp.Body).Decode(&configsResp); err != nil {
		return nil, fmt.Errorf("failed to decode client configs: %w", err)
	}

	if configsResp.AuthorizationEndpoint == "" {
		return nil, fmt.Errorf("authorization_endpoint not found in client configs")
	}
	if configsResp.TokenEndpoint == "" {
		return nil, fmt.Errorf("token_endpoint not found in client configs")
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
			}
			break
		}
	}

	if cliClient == nil {
		return nil, fmt.Errorf("CLI client configuration (name='cli') not found in external_clients")
	}

	return cliClient, nil
}
