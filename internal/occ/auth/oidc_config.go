// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// protectedResourceResponse represents the extended /.well-known/oauth-protected-resource response
type protectedResourceResponse struct {
	AuthorizationServers      []string     `json:"authorization_servers"`
	OpenChoreoClients         []clientInfo `json:"openchoreo_clients"`
	OpenChoreoSecurityEnabled bool         `json:"openchoreo_security_enabled"`
}

// clientInfo holds OAuth client configuration for an OpenChoreo integration
type clientInfo struct {
	Name     string   `json:"name"`
	ClientID string   `json:"client_id"`
	Scopes   []string `json:"scopes"`
}

// oidcProviderDiscovery holds the relevant fields from an OIDC provider's discovery document
type oidcProviderDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JwksURI               string `json:"jwks_uri,omitempty"`
}

// FetchOIDCConfig fetches CLI client configuration using a two-step discovery process:
//  1. Calls /.well-known/oauth-protected-resource on the OpenChoreo API to discover the
//     authorization server URL and OpenChoreo-specific client configurations.
//  2. Calls the authorization server's /.well-known/openid-configuration to discover
//     the authorization and token endpoints (standard OIDC discovery per RFC 8414).
func FetchOIDCConfig(apiURL string) (*OIDCConfig, error) {
	resource, err := fetchProtectedResource(apiURL)
	if err != nil {
		return nil, err
	}

	if len(resource.AuthorizationServers) == 0 {
		return nil, fmt.Errorf("no authorization_servers found in oauth-protected-resource metadata")
	}
	issuer := resource.AuthorizationServers[0]

	var cliClient *clientInfo
	for i := range resource.OpenChoreoClients {
		if resource.OpenChoreoClients[i].Name == "cli" {
			cliClient = &resource.OpenChoreoClients[i]
			break
		}
	}
	if cliClient == nil {
		return nil, fmt.Errorf("CLI client configuration (name='cli') not found in openchoreo_clients")
	}

	provider, err := fetchOIDCProviderDiscovery(issuer)
	if err != nil {
		return nil, err
	}

	return &OIDCConfig{
		AuthorizationEndpoint: provider.AuthorizationEndpoint,
		TokenEndpoint:         provider.TokenEndpoint,
		ClientID:              cliClient.ClientID,
		Scopes:                cliClient.Scopes,
		SecurityEnabled:       resource.OpenChoreoSecurityEnabled,
		Issuer:                issuer,
		JwksURI:               provider.JwksURI,
	}, nil
}

func fetchProtectedResource(apiURL string) (*protectedResourceResponse, error) {
	req, err := http.NewRequest(
		http.MethodGet,
		strings.TrimSuffix(apiURL, "/")+"/.well-known/oauth-protected-resource",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch oauth-protected-resource metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("oauth-protected-resource request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var resource protectedResourceResponse
	if err := json.NewDecoder(resp.Body).Decode(&resource); err != nil {
		return nil, fmt.Errorf("failed to decode oauth-protected-resource metadata: %w", err)
	}

	return &resource, nil
}

func fetchOIDCProviderDiscovery(issuer string) (*oidcProviderDiscovery, error) {
	req, err := http.NewRequest(
		http.MethodGet,
		strings.TrimSuffix(issuer, "/")+"/.well-known/openid-configuration",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC discovery request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OIDC provider discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OIDC discovery request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var discovery oidcProviderDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return nil, fmt.Errorf("failed to decode OIDC discovery document: %w", err)
	}

	if discovery.AuthorizationEndpoint == "" {
		return nil, fmt.Errorf("authorization_endpoint not found in OIDC discovery document")
	}
	if discovery.TokenEndpoint == "" {
		return nil, fmt.Errorf("token_endpoint not found in OIDC discovery document")
	}

	return &discovery, nil
}
