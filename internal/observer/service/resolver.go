// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
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

// cacheEntry holds a cached UID with its expiration time
type cacheEntry struct {
	uid       string
	expiresAt time.Time
}

// tokenCache holds the OAuth2 access token with its expiration
type tokenCache struct {
	token     string
	expiresAt time.Time
}

// ResourceResolver provides methods to resolve resource names to UIDs
// by calling the openchoreo-api with OAuth2 client credentials authentication.
// It includes in-memory caching with TTL to reduce API calls.
type ResourceResolver struct {
	config     *config.ResolverConfig
	httpClient *http.Client
	logger     *slog.Logger

	// Token cache (thread-safe)
	tokenMu    sync.RWMutex
	tokenEntry *tokenCache

	// UID cache (thread-safe)
	cache sync.Map // map[string]cacheEntry
}

// NewResourceResolver creates a new ResourceResolver instance
func NewResourceResolver(cfg *config.ResolverConfig, logger *slog.Logger) *ResourceResolver {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: cfg.TLSInsecureSkipVerify, //nolint:gosec // G402: Configurable for development
		},
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &ResourceResolver{
		config: cfg,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
		logger: logger,
	}
}

// GetNamespaceUID resolves a namespace name to its UID
func (r *ResourceResolver) GetNamespaceUID(namespaceName string) string {
	if namespaceName == "" {
		return ""
	}

	cacheKey := fmt.Sprintf("namespace:%s", namespaceName)
	if uid := r.getFromCache(cacheKey); uid != "" {
		return uid
	}

	// Call API: GET /api/v1/namespaces/{namespaceName}
	path := fmt.Sprintf("/api/v1/namespaces/%s", url.PathEscape(namespaceName))
	uid, err := r.fetchResourceUID(path)
	if err != nil {
		r.logger.Warn("Failed to resolve namespace UID, using name as fallback",
			"namespace", namespaceName,
			"error", err)
		return namespaceName
	}

	r.setInCache(cacheKey, uid)
	return uid
}

// GetProjectUID resolves a project name to its UID within a namespace
func (r *ResourceResolver) GetProjectUID(namespaceName, projectName string) string {
	if projectName == "" {
		return ""
	}

	cacheKey := fmt.Sprintf("project:%s/%s", namespaceName, projectName)
	if uid := r.getFromCache(cacheKey); uid != "" {
		return uid
	}

	// Call API: GET /api/v1/namespaces/{ns}/projects/{projectName}
	path := fmt.Sprintf("/api/v1/namespaces/%s/projects/%s",
		url.PathEscape(namespaceName),
		url.PathEscape(projectName))
	uid, err := r.fetchResourceUID(path)
	if err != nil {
		r.logger.Warn("Failed to resolve project UID, using name as fallback",
			"namespace", namespaceName,
			"project", projectName,
			"error", err)
		return projectName
	}

	r.setInCache(cacheKey, uid)
	return uid
}

// GetComponentUID resolves a component name to its UID within a namespace and project
func (r *ResourceResolver) GetComponentUID(namespaceName, projectName, componentName string) string {
	if componentName == "" {
		return ""
	}

	cacheKey := fmt.Sprintf("component:%s/%s/%s", namespaceName, projectName, componentName)
	if uid := r.getFromCache(cacheKey); uid != "" {
		return uid
	}

	// Call API: GET /api/v1/namespaces/{ns}/projects/{proj}/components/{componentName}
	path := fmt.Sprintf("/api/v1/namespaces/%s/projects/%s/components/%s",
		url.PathEscape(namespaceName),
		url.PathEscape(projectName),
		url.PathEscape(componentName))
	uid, err := r.fetchResourceUID(path)
	if err != nil {
		r.logger.Warn("Failed to resolve component UID, using name as fallback",
			"namespace", namespaceName,
			"project", projectName,
			"component", componentName,
			"error", err)
		return componentName
	}

	r.setInCache(cacheKey, uid)
	return uid
}

// GetEnvironmentUID resolves an environment name to its UID within a namespace
func (r *ResourceResolver) GetEnvironmentUID(namespaceName, environmentName string) string {
	if environmentName == "" {
		return ""
	}

	cacheKey := fmt.Sprintf("environment:%s/%s", namespaceName, environmentName)
	if uid := r.getFromCache(cacheKey); uid != "" {
		return uid
	}

	// Call API: GET /api/v1/namespaces/{ns}/environments/{environmentName}
	path := fmt.Sprintf("/api/v1/namespaces/%s/environments/%s",
		url.PathEscape(namespaceName),
		url.PathEscape(environmentName))
	uid, err := r.fetchResourceUID(path)
	if err != nil {
		r.logger.Warn("Failed to resolve environment UID, using name as fallback",
			"namespace", namespaceName,
			"environment", environmentName,
			"error", err)
		return environmentName
	}

	r.setInCache(cacheKey, uid)
	return uid
}

// fetchResourceUID makes an HTTP GET request to the openchoreo-api and extracts data.uid
func (r *ResourceResolver) fetchResourceUID(path string) (string, error) {
	// Skip API call if not configured
	if r.config.OpenChoreoAPIURL == "" {
		return "", fmt.Errorf("openchoreo API URL not configured")
	}

	// Get access token
	token, err := r.getAccessToken()
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}

	// Build request URL
	reqURL := strings.TrimSuffix(r.config.OpenChoreoAPIURL, "/") + path

	ctx, cancel := context.WithTimeout(context.Background(), r.config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to extract data.uid
	var response struct {
		Data struct {
			UID string `json:"uid"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Data.UID == "" {
		return "", fmt.Errorf("uid not found in response")
	}

	r.logger.Debug("Resolved resource UID",
		"path", path,
		"uid", response.Data.UID)

	return response.Data.UID, nil
}

// getAccessToken returns a valid OAuth2 access token, fetching a new one if needed
func (r *ResourceResolver) getAccessToken() (string, error) {
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

	token, expiresIn, err := r.fetchAccessToken()
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
func (r *ResourceResolver) fetchAccessToken() (string, time.Duration, error) {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", r.config.OAuthClientID)
	data.Set("client_secret", r.config.OAuthClientSecret)

	ctx, cancel := context.WithTimeout(context.Background(), r.config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.config.OAuthTokenURL, strings.NewReader(data.Encode()))
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

// getFromCache retrieves a UID from the cache if it exists and hasn't expired
func (r *ResourceResolver) getFromCache(key string) string {
	if value, ok := r.cache.Load(key); ok {
		entry := value.(cacheEntry)
		if time.Now().Before(entry.expiresAt) {
			r.logger.Debug("Cache hit", "key", key, "uid", entry.uid)
			return entry.uid
		}
		// Entry expired, delete it
		r.cache.Delete(key)
	}
	return ""
}

// setInCache stores a UID in the cache with the configured TTL
func (r *ResourceResolver) setInCache(key, uid string) {
	ttl := r.config.CacheTTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	r.cache.Store(key, cacheEntry{
		uid:       uid,
		expiresAt: time.Now().Add(ttl),
	})
	r.logger.Debug("Cached UID", "key", key, "uid", uid, "ttl", ttl)
}
