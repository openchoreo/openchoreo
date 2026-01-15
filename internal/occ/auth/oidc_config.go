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

// OIDCConfig represents the OIDC discovery response
type OIDCConfig struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	CLIClientID           string `json:"cli_client_id"`
}

// FetchOIDCConfig fetches OIDC configuration including both endpoints and CLI client ID
func FetchOIDCConfig(apiURL string) (*OIDCConfig, error) {
	req, err := http.NewRequest(
		http.MethodGet,
		apiURL+"/api/v1/oidc-config",
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

	var oidcConfig OIDCConfig
	if err := json.NewDecoder(resp.Body).Decode(&oidcConfig); err != nil {
		return nil, fmt.Errorf("failed to decode OIDC config: %w", err)
	}

	if oidcConfig.AuthorizationEndpoint == "" {
		return nil, fmt.Errorf("authorization endpoint not found in OIDC config")
	}
	if oidcConfig.TokenEndpoint == "" {
		return nil, fmt.Errorf("token endpoint not found in OIDC config")
	}
	if oidcConfig.CLIClientID == "" {
		return nil, fmt.Errorf("cli_client_id not found in OIDC config")
	}

	return &oidcConfig, nil
}
