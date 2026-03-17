// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
)

// tokenCache holds the OAuth2 access token with its expiration
type tokenCache struct {
	token     string
	expiresAt time.Time
}

var (
	ErrResourceNotFound = errors.New("resource not found")
	ErrScopeAuthFailed  = errors.New("observer scope resolution auth failed")
)

// ResourceUIDResolver provides methods to resolve resource names to UIDs
// by calling the openchoreo-api with OAuth2 client credentials authentication.
type ResourceUIDResolver struct {
	config     *config.UIDResolverConfig
	httpClient *http.Client
	logger     *slog.Logger

	// Token cache (thread-safe)
	tokenMu    sync.RWMutex
	tokenEntry *tokenCache
}

// NewResourceUIDResolver creates a new ResourceUIDResolver instance
func NewResourceUIDResolver(cfg *config.UIDResolverConfig, logger *slog.Logger) *ResourceUIDResolver {
	if cfg == nil {
		cfg = &config.UIDResolverConfig{}
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: cfg.TLSInsecureSkipVerify, //nolint:gosec // G402: Configurable for development
		},
	}

	return &ResourceUIDResolver{
		config: cfg,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   cfg.Timeout,
		},
		logger: logger,
	}
}

// GetNamespaceUID resolves a namespace name to its UID.
func (r *ResourceUIDResolver) GetNamespaceUID(ctx context.Context, namespaceName string) (string, error) {
	if namespaceName == "" {
		return "", nil
	}

	// Call API: GET /api/v1/namespaces/{namespaceName}
	path := fmt.Sprintf("/api/v1/namespaces/%s", url.PathEscape(namespaceName))
	uid, err := r.fetchResourceUID(ctx, path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve namespace UID for namespace %q: %w", namespaceName, err)
	}

	return uid, nil
}

// GetProjectUID resolves a project name to its UID within a namespace.
func (r *ResourceUIDResolver) GetProjectUID(ctx context.Context, namespaceName, projectName string) (string, error) {
	if projectName == "" {
		return "", nil
	}

	// Call API: GET /api/v1/namespaces/{ns}/projects/{projectName}
	path := fmt.Sprintf("/api/v1/namespaces/%s/projects/%s",
		url.PathEscape(namespaceName),
		url.PathEscape(projectName))
	uid, err := r.fetchResourceUID(ctx, path)
	if err != nil {
		return "", fmt.Errorf(
			"failed to resolve project UID for namespace %q project %q: %w",
			namespaceName,
			projectName,
			err,
		)
	}

	return uid, nil
}

// GetComponentUID resolves a component name to its UID within a namespace and project.
func (r *ResourceUIDResolver) GetComponentUID(
	ctx context.Context,
	namespaceName, projectName, componentName string,
) (string, error) {
	if componentName == "" {
		return "", nil
	}

	// Call API: GET /api/v1/namespaces/{ns}/components/{componentName}
	path := fmt.Sprintf("/api/v1/namespaces/%s/components/%s",
		url.PathEscape(namespaceName),
		url.PathEscape(componentName))
	uid, err := r.fetchResourceUID(ctx, path)
	if err != nil {
		return "", fmt.Errorf(
			"failed to resolve component UID for namespace %q project %q component %q: %w",
			namespaceName,
			projectName,
			componentName,
			err,
		)
	}

	return uid, nil
}

// GetEnvironmentUID resolves an environment name to its UID within a namespace.
func (r *ResourceUIDResolver) GetEnvironmentUID(ctx context.Context, namespaceName, environmentName string) (string, error) {
	if environmentName == "" {
		return "", nil
	}

	// Call API: GET /api/v1/namespaces/{ns}/environments/{environmentName}
	path := fmt.Sprintf("/api/v1/namespaces/%s/environments/%s",
		url.PathEscape(namespaceName),
		url.PathEscape(environmentName))
	uid, err := r.fetchResourceUID(ctx, path)
	if err != nil {
		return "", fmt.Errorf(
			"failed to resolve environment UID for namespace %q environment %q: %w",
			namespaceName,
			environmentName,
			err,
		)
	}

	return uid, nil
}

// fetchResourceUID makes an HTTP GET request to the openchoreo-api and extracts data.uid
func (r *ResourceUIDResolver) fetchResourceUID(ctx context.Context, path string) (string, error) {
	// Skip API call if not configured
	if r.config.OpenChoreoAPIURL == "" {
		return "", fmt.Errorf("openchoreo API URL not configured")
	}

	// Build request URL
	reqURL := strings.TrimSuffix(r.config.OpenChoreoAPIURL, "/") + path
	for attempt := 0; attempt < (r.config.MaxAuthRetry + 1); attempt++ {
		token, err := r.getAccessToken(ctx)
		if err != nil {
			return "", fmt.Errorf("%w: failed to obtain access token: %w", ErrScopeAuthFailed, err)
		}

		reqCtx, reqCancel := context.WithTimeout(ctx, r.config.Timeout)

		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, reqURL, nil)
		if err != nil {
			reqCancel()
			return "", fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := r.httpClient.Do(req)
		reqCancel()
		if err != nil {
			return "", fmt.Errorf("request failed: %w", err)
		}

		switch resp.StatusCode {
		case http.StatusOK:
			// Parse response to extract metadata.uid
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				resp.Body.Close()
				return "", fmt.Errorf("failed to read response body: %w", err)
			}
			resp.Body.Close()

			r.logger.Debug("Raw UID resolver response", "path", path, "status", resp.StatusCode, "body", string(body))

			var response struct {
				Metadata struct {
					UID string `json:"uid"`
				} `json:"metadata"`
			}

			if err := json.Unmarshal(body, &response); err != nil {
				return "", fmt.Errorf("failed to decode response: %w", err)
			}

			if response.Metadata.UID == "" {
				return "", fmt.Errorf("uid not found in response")
			}

			r.logger.Debug("Resolved resource UID",
				"path", path,
				"uid", response.Metadata.UID)

			return response.Metadata.UID, nil

		case http.StatusNotFound:
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return "", fmt.Errorf("%w: %s", ErrResourceNotFound, path)

		case http.StatusUnauthorized:
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()

			r.tokenMu.Lock()
			r.tokenEntry = nil
			r.tokenMu.Unlock()

			remaining := r.config.MaxAuthRetry - attempt
			if remaining > 0 {
				r.logger.Debug("Received 401 from openchoreo-api; invalidating cached token and retrying",
					"path", path, "attempt", attempt+1, "remaining_retries", remaining)
				continue
			}

			r.logger.Error("Received 401 from openchoreo-api and retries are exhausted",
				"path", path, "max_auth_retry", r.config.MaxAuthRetry)
			return "", fmt.Errorf("%w: received 401 after %d attempt(s)", ErrScopeAuthFailed, r.config.MaxAuthRetry+1)

		default:
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return "", fmt.Errorf("API returned status %d", resp.StatusCode)
		}
	}
	// Unreachable: every loop iteration either returns or continues (401 retry path).
	// Kept as a defensive fallback.
	return "", fmt.Errorf("%w: retry loop exhausted", ErrScopeAuthFailed)
}

// getAccessToken returns a valid OAuth2 access token, fetching a new one if needed
func (r *ResourceUIDResolver) getAccessToken(ctx context.Context) (string, error) {
	// If OAuth is not configured, return empty token (API might not require auth)
	if r.config.OAuthTokenURL == "" || r.config.OAuthClientID == "" {
		return "", nil
	}

	// Check cached token
	r.tokenMu.RLock()
	if r.tokenEntry != nil && time.Now().Before(r.tokenEntry.expiresAt) {
		token := r.tokenEntry.token
		r.tokenMu.RUnlock()
		return token, nil
	}
	r.tokenMu.RUnlock()

	// Fetch new token
	r.tokenMu.Lock()
	defer r.tokenMu.Unlock()

	// Double-check after acquiring write lock
	if r.tokenEntry != nil && time.Now().Before(r.tokenEntry.expiresAt) {
		return r.tokenEntry.token, nil
	}

	token, expiresIn, err := r.fetchAccessToken(ctx)
	if err != nil {
		return "", err
	}

	// Cache token with some buffer before expiry
	expiryBuffer := time.Duration(float64(expiresIn) * 0.9)
	r.tokenEntry = &tokenCache{
		token:     token,
		expiresAt: time.Now().Add(expiryBuffer),
	}

	r.logger.Debug("Fetched new OAuth2 access token", "expires_in", expiresIn)

	return token, nil
}

// fetchAccessToken performs the OAuth2 client credentials grant
func (r *ResourceUIDResolver) fetchAccessToken(ctx context.Context) (string, time.Duration, error) {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", r.config.OAuthClientID)
	data.Set("client_secret", r.config.OAuthClientSecret)

	reqCtx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, r.config.OAuthTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", 0, fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", 0, fmt.Errorf("empty access token in response")
	}

	expiresIn := time.Duration(tokenResp.ExpiresIn) * time.Second
	if expiresIn == 0 {
		expiresIn = 1 * time.Hour // Default to 1 hour if not specified
	}

	return tokenResp.AccessToken, expiresIn, nil
}
