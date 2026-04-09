// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// authRoundTripper injects a Bearer token into every request.
type authRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (t *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}

// authedHTTPClient creates an HTTP client that injects a Bearer token.
// No global Timeout is set because streaming uses long-lived connections;
// per-request timeouts are controlled via context instead.
func authedHTTPClient(token string, tlsInsecure bool) *http.Client {
	base := newTransport(tlsInsecure)
	return &http.Client{
		Transport: &authRoundTripper{token: token, base: base},
	}
}

func newTransport(tlsInsecure bool) *http.Transport {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t = &http.Transport{}
	}
	t = t.Clone() //nolint:gosec // MinVersion set below
	t.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if tlsInsecure {
		t.TLSClientConfig.InsecureSkipVerify = true //nolint:gosec // intentional for self-signed certs
	}
	return t
}

// tokenCache caches an OAuth2 access token with expiry and coalesces
// concurrent fetches to avoid thundering herd on the token endpoint.
type tokenCache struct {
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func (c *tokenCache) getUnlocked() (string, bool) {
	if c.token != "" && time.Now().Before(c.expiresAt) {
		return c.token, true
	}
	return "", false
}

func (c *tokenCache) setUnlocked(token string, expiresIn int) {
	c.token = token
	ttl := time.Duration(expiresIn) * time.Second
	if ttl > 30*time.Second {
		ttl -= 30 * time.Second
	}
	c.expiresAt = time.Now().Add(ttl)
}

var oauth2Cache = &tokenCache{}

// fetchOAuth2Token performs an OAuth2 client_credentials token fetch.
// Caches the token and reuses it until expired.
func fetchOAuth2Token(ctx context.Context, tokenURL, clientID, clientSecret string, tlsInsecure bool) (string, error) {
	// Hold lock to coalesce concurrent fetches (double-checked locking).
	oauth2Cache.mu.Lock()
	defer oauth2Cache.mu.Unlock()
	if token, ok := oauth2Cache.getUnlocked(); ok {
		return token, nil
	}

	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}

	client := &http.Client{Timeout: 15 * time.Second, Transport: newTransport(tlsInsecure)}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching OAuth2 token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OAuth2 token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("OAuth2 token response has empty access_token")
	}

	if tokenResp.ExpiresIn > 0 {
		oauth2Cache.setUnlocked(tokenResp.AccessToken, tokenResp.ExpiresIn)
	}

	return tokenResp.AccessToken, nil
}
