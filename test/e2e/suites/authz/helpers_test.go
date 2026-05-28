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

const (
	apiURL   = "http://api.e2e-cp.local:28080"
	tokenURL = "http://thunder.e2e-cp.local:28080/oauth2/token"

	// Admin identity: service_mcp_client has a bootstrap admin ClusterAuthzRoleBinding.
	adminClientID     = "service_mcp_client"
	adminClientSecret = "service_mcp_client_secret"

	// Test subject identity: customer-portal-client has NO bootstrap binding.
	subjectClientID     = "customer-portal-client"
	subjectClientSecret = "supersecret"
)

var testNs = fmt.Sprintf("e2e-authz-%d", time.Now().UnixNano())

func labelSelector() string {
	return fmt.Sprintf("e2e-authz/run=%s", testNs)
}

func testLabel() map[string]string {
	return map[string]string{"e2e-authz/run": testNs}
}

func bearerAuth(token string) gen.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

func newAPIClient(token string) *gen.ClientWithResponses {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	client, err := gen.NewClientWithResponses(
		apiURL,
		gen.WithHTTPClient(httpClient),
		gen.WithRequestEditorFn(bearerAuth(token)),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to create API client: %v", err))
	}
	return client
}

func newUnauthClient() *gen.ClientWithResponses {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	client, err := gen.NewClientWithResponses(
		apiURL,
		gen.WithHTTPClient(httpClient),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to create unauthenticated API client: %v", err))
	}
	return client
}

// fetchAdminToken obtains a token for service_mcp_client using client_secret_basic (HTTP Basic auth).
func fetchAdminToken() (string, error) {
	data := url.Values{
		"grant_type": {"client_credentials"},
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		tokenURL,
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(adminClientID, adminClientSecret)

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
