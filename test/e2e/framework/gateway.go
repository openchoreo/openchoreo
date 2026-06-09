// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	"github.com/onsi/gomega"
)

// AssertHTTPRouteAccepted asserts that the named HTTPRoute has at least one
// parent reporting Accepted=True in status.parents[].conditions.
func AssertHTTPRouteAccepted(g gomega.Gomega, kubeContext, namespace, name string) {
	out, err := KubectlGetJsonpath(
		kubeContext, namespace,
		"httproute.gateway.networking.k8s.io", name,
		`{.status.parents[0].conditions[?(@.type=="Accepted")].status}`,
	)
	g.Expect(err).NotTo(gomega.HaveOccurred(),
		"failed to read HTTPRoute %s/%s accepted condition", namespace, name)
	g.Expect(out).To(gomega.Equal("True"),
		"HTTPRoute %s/%s should be Accepted", namespace, name)
}

// CountHTTPRoutesByLabel returns the number of HTTPRoutes matching a label selector.
func CountHTTPRoutesByLabel(kubeContext, namespace, labelSelector string) (int, error) {
	out, err := Kubectl(kubeContext,
		"get", "httproute.gateway.networking.k8s.io",
		"-n", namespace,
		"-l", labelSelector,
		"-o", "name",
	)
	if err != nil {
		return 0, fmt.Errorf("failed to list httproutes in %s with selector %s: %w", namespace, labelSelector, err)
	}
	if strings.TrimSpace(out) == "" {
		return 0, nil
	}
	return len(strings.Split(strings.TrimSpace(out), "\n")), nil
}

// GetHTTPRouteNames returns the names of HTTPRoutes matching a label selector.
func GetHTTPRouteNames(kubeContext, namespace, labelSelector string) ([]string, error) {
	out, err := Kubectl(kubeContext,
		"get", "httproute.gateway.networking.k8s.io",
		"-n", namespace,
		"-l", labelSelector,
		"-o", "jsonpath={.items[*].metadata.name}",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list httproutes in %s with selector %s: %w", namespace, labelSelector, err)
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil, nil
	}
	return strings.Fields(trimmed), nil
}

// WaitForHTTP polls a URL until it returns HTTP 200 or the timeout expires.
// Use this to verify that a service is reachable through the external gateway
// before running tests that depend on it.
func WaitForHTTP(targetURL string, timeout time.Duration) {
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(targetURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Fprintf(GinkgoWriter, "gateway ready: %s\n", targetURL)
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	Fail(fmt.Sprintf("gateway not reachable at %s within %s", targetURL, timeout))
}

// FetchClientCredentialsToken obtains an access token via the OAuth2
// client_credentials grant from the given token endpoint.
func FetchClientCredentialsToken(tokenEndpoint, clientID, clientSecret string) (string, error) {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(tokenEndpoint, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("token request to %s failed: %w", tokenEndpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("missing access_token in token response: %s", string(body))
	}
	return tokenResp.AccessToken, nil
}
