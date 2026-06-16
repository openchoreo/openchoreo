// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// OpenChoreo API access used by the build-logs API spec. Mirrors the helpers in
// the openchoreoapi suite; each suite is its own package so the helpers cannot
// be shared and are kept intentionally minimal here (only what this suite needs).
const (
	apiURL   = "http://api.e2e-cp.local:28080"
	tokenURL = "http://thunder.e2e-cp.local:28080/oauth2/token"

	tokenClientID     = "service_mcp_client"
	tokenClientSecret = "service_mcp_client_secret"
)

// bearerAuth returns a RequestEditorFn that injects the Authorization header.
func bearerAuth(token string) gen.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

// newAPIClient creates a gen.ClientWithResponses that talks to the API server
// with Bearer token authentication.
func newAPIClient(token string) (*gen.ClientWithResponses, error) {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	return gen.NewClientWithResponses(
		apiURL,
		gen.WithHTTPClient(httpClient),
		gen.WithRequestEditorFn(bearerAuth(token)),
	)
}

// fetchToken obtains a JWT access token from Thunder IdP using client_secret_basic
// (HTTP Basic auth), matching the service_mcp_client configuration.
func fetchToken() (string, error) {
	data := url.Values{"grant_type": {"client_credentials"}}

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(tokenClientID, tokenClientSecret)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token request returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access_token in response")
	}
	return tokenResp.AccessToken, nil
}

// createGitBasicAuthSecret provisions a kubernetes.io/basic-auth secret through
// the OpenChoreo Secret API. The API writes the K8s Secret plus a PushSecret to
// the workflow plane and pushes the values into its ClusterSecretStore
// (OpenBao), then creates a SecretReference named secretName in the control
// plane that the build's repository.secretRef resolves. Targeting the workflow
// plane (rather than seeding OpenBao directly) is what makes the private-repo
// build work in both single- and multi-cluster topologies, since the build runs
// in the workflow plane and reads its credentials from that plane's store.
func createGitBasicAuthSecret(namespace, secretName, username, pat string) error {
	token, err := fetchToken()
	if err != nil {
		return fmt.Errorf("fetch token: %w", err)
	}
	client, err := newAPIClient(token)
	if err != nil {
		return fmt.Errorf("new api client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.CreateSecretWithResponse(ctx, namespace, gen.CreateSecretRequest{
		SecretName: secretName,
		SecretType: gen.SecretTypeKubernetesIobasicAuth,
		TargetPlane: gen.TargetPlaneRef{
			Kind: gen.TargetPlaneRefKindClusterWorkflowPlane,
			Name: "default",
		},
		Data: map[string]string{
			"username": username,
			"password": pat,
		},
	})
	if err != nil {
		return fmt.Errorf("create secret request: %w", err)
	}
	if resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("create secret returned %d: %s", resp.StatusCode(), string(resp.Body))
	}
	return nil
}
