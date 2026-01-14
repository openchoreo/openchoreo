// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	configContext "github.com/openchoreo/openchoreo/pkg/cli/cmd/config"
)

// APIClient provides HTTP client for OpenChoreo API server
type APIClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// ApplyResponse represents the response from /api/v1/apply
type ApplyResponse struct {
	Success bool `json:"success"`
	Data    struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Name       string `json:"name"`
		Namespace  string `json:"namespace,omitempty"`
		Operation  string `json:"operation"` // "created" or "updated"
	} `json:"data"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

type DeleteResponse struct {
	Success bool `json:"success"`
	Data    struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Name       string `json:"name"`
		Namespace  string `json:"namespace,omitempty"`
		Operation  string `json:"operation"` // "deleted" or "not_found"
	} `json:"data"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

// OrganizationResponse represents an organization from the API
type OrganizationResponse struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
}

// ListResponse represents a paginated list response
type ListResponse struct {
	Items      []OrganizationResponse `json:"items"`
	TotalCount int                    `json:"totalCount"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"pageSize"`
}

// ListOrganizationsResponse represents the response from listing organizations
type ListOrganizationsResponse struct {
	Success bool         `json:"success"`
	Data    ListResponse `json:"data"`
	Error   string       `json:"error,omitempty"`
	Code    string       `json:"code,omitempty"`
}

// ProjectResponse represents a project from the API
type ProjectResponse struct {
	Name               string `json:"name"`
	OrgName            string `json:"orgName"`
	DisplayName        string `json:"displayName,omitempty"`
	Description        string `json:"description,omitempty"`
	DeploymentPipeline string `json:"deploymentPipeline,omitempty"`
	CreatedAt          string `json:"createdAt"`
	Status             string `json:"status,omitempty"`
}

// ListProjectsResponse represents the response from listing projects
type ListProjectsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Items      []ProjectResponse `json:"items"`
		TotalCount int               `json:"totalCount"`
		Page       int               `json:"page"`
		PageSize   int               `json:"pageSize"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

// ComponentResponse represents a component from the API
type ComponentResponse struct {
	Name        string `json:"name"`
	OrgName     string `json:"orgName"`
	ProjectName string `json:"projectName"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	CreatedAt   string `json:"createdAt"`
	Status      string `json:"status,omitempty"`
}

// ListComponentsResponse represents the response from listing components
type ListComponentsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Items      []ComponentResponse `json:"items"`
		TotalCount int                 `json:"totalCount"`
		Page       int                 `json:"page"`
		PageSize   int                 `json:"pageSize"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

// NewAPIClient creates a new API client with control plane auto-detection
func NewAPIClient() (*APIClient, error) {
	storedCfg, err := config.LoadStoredConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if storedCfg.CurrentContext == "" {
		return nil, fmt.Errorf("no current context set")
	}

	// Find current context
	var currentContext *configContext.Context
	for idx := range storedCfg.Contexts {
		if storedCfg.Contexts[idx].Name == storedCfg.CurrentContext {
			currentContext = &storedCfg.Contexts[idx]
			break
		}
	}

	if currentContext == nil {
		return nil, fmt.Errorf("current context '%s' not found", storedCfg.CurrentContext)
	}

	// Find control plane
	var controlPlane *configContext.ControlPlane
	for idx := range storedCfg.ControlPlanes {
		if storedCfg.ControlPlanes[idx].Name == currentContext.ControlPlane {
			controlPlane = &storedCfg.ControlPlanes[idx]
			break
		}
	}

	if controlPlane == nil {
		return nil, fmt.Errorf("control plane '%s' not found", currentContext.ControlPlane)
	}

	// Find credential and get token
	token := ""
	if currentContext.Credentials != "" {
		for idx := range storedCfg.Credentials {
			if storedCfg.Credentials[idx].Name == currentContext.Credentials {
				token = storedCfg.Credentials[idx].Token
				break
			}
		}
	}

	return &APIClient{
		baseURL:    controlPlane.URL,
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// HealthCheck verifies API server connectivity
func (c *APIClient) HealthCheck(ctx context.Context) error {
	resp, err := c.get(ctx, "/health")
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// Apply sends a resource to the /api/v1/apply endpoint
func (c *APIClient) Apply(ctx context.Context, resource map[string]interface{}) (*ApplyResponse, error) {
	resp, err := c.post(ctx, "/api/v1/apply", resource)
	if err != nil {
		return nil, fmt.Errorf("failed to make apply request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var applyResp ApplyResponse
	if err := json.Unmarshal(body, &applyResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !applyResp.Success {
		return &applyResp, fmt.Errorf("apply failed: %s", applyResp.Error)
	}

	return &applyResp, nil
}

func (c *APIClient) Delete(ctx context.Context, resource map[string]interface{}) (*DeleteResponse, error) {
	resp, err := c.delete(ctx, "/api/v1/delete", resource)
	if err != nil {
		return nil, fmt.Errorf("failed to make delete request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var deleteResp DeleteResponse
	if err := json.Unmarshal(body, &deleteResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w\nResponse body: %s", err, string(body))
	}

	if !deleteResp.Success {
		return &deleteResp, fmt.Errorf("delete failed: %s", deleteResp.Error)
	}

	return &deleteResp, nil
}

// ListOrganizations retrieves all organizations from the API
func (c *APIClient) ListOrganizations(ctx context.Context) ([]OrganizationResponse, error) {
	resp, err := c.get(ctx, "/api/v1/orgs")
	if err != nil {
		return nil, fmt.Errorf("failed to make list organizations request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var listResp ListOrganizationsResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !listResp.Success {
		return nil, fmt.Errorf("list organizations failed: %s", listResp.Error)
	}

	return listResp.Data.Items, nil
}

// ListProjects retrieves all projects for an organization from the API
func (c *APIClient) ListProjects(ctx context.Context, orgName string) ([]ProjectResponse, error) {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects", orgName)
	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to make list projects request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var listResp ListProjectsResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !listResp.Success {
		return nil, fmt.Errorf("list projects failed: %s", listResp.Error)
	}

	return listResp.Data.Items, nil
}

// ListComponents retrieves all components for an organization and project from the API
func (c *APIClient) ListComponents(ctx context.Context, orgName, projectName string) ([]ComponentResponse, error) {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/components", orgName, projectName)
	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to make list components request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var listResp ListComponentsResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !listResp.Success {
		return nil, fmt.Errorf("list components failed: %s", listResp.Error)
	}

	return listResp.Data.Items, nil
}

// GetComponentTypeSchema fetches ComponentType schema from the API
func (c *APIClient) GetComponentTypeSchema(ctx context.Context, orgName, ctName string) (*json.RawMessage, error) {
	path := fmt.Sprintf("/api/v1/orgs/%s/component-types/%s/schema", orgName, ctName)
	return c.getSchema(ctx, path)
}

// GetTraitSchema fetches Trait schema from the API
func (c *APIClient) GetTraitSchema(ctx context.Context, orgName, traitName string) (*json.RawMessage, error) {
	path := fmt.Sprintf("/api/v1/orgs/%s/traits/%s/schema", orgName, traitName)
	return c.getSchema(ctx, path)
}

// GetComponentWorkflowSchema fetches ComponentWorkflow schema from the API
func (c *APIClient) GetComponentWorkflowSchema(ctx context.Context, orgName, cwName string) (*json.RawMessage, error) {
	path := fmt.Sprintf("/api/v1/orgs/%s/component-workflows/%s/schema", orgName, cwName)
	return c.getSchema(ctx, path)
}

// getSchema is a helper to fetch schema from the API
func (c *APIClient) getSchema(ctx context.Context, path string) (*json.RawMessage, error) {
	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Handle non-OK status codes
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse wrapped API response
	var apiResponse struct {
		Success bool             `json:"success"`
		Data    *json.RawMessage `json:"data"`
		Error   string           `json:"error,omitempty"`
		Code    string           `json:"code,omitempty"`
	}
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("invalid API response format: %w", err)
	}

	if !apiResponse.Success {
		if apiResponse.Code != "" {
			return nil, fmt.Errorf("%s (error code: %s)", apiResponse.Error, apiResponse.Code)
		}
		return nil, fmt.Errorf("%s", apiResponse.Error)
	}

	return apiResponse.Data, nil
}

// Get performs a GET request to the API
func (c *APIClient) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.doRequest(ctx, "GET", path, nil)
}

// HTTP helper methods
func (c *APIClient) get(ctx context.Context, path string) (*http.Response, error) {
	return c.doRequest(ctx, "GET", path, nil)
}

func (c *APIClient) post(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	return c.doRequest(ctx, "POST", path, body)
}

func (c *APIClient) delete(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	return c.doRequest(ctx, "DELETE", path, body)
}

func (c *APIClient) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	// Check if token needs refresh before making request
	if c.token != "" && c.isTokenExpired() {
		if err := c.refreshToken(); err != nil {
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
	}

	url := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// isTokenExpired checks if the JWT token is expired or will expire soon (within 1 minute)
func (c *APIClient) isTokenExpired() bool {
	if c.token == "" {
		return false
	}

	// Parse JWT token (format: header.payload.signature)
	parts := strings.Split(c.token, ".")
	if len(parts) != 3 {
		return true // Invalid token format
	}

	// Decode payload (base64url)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return true // Failed to decode
	}

	// Parse payload JSON
	var claims struct {
		Exp int64 `json:"exp"` // Expiry time as Unix timestamp
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return true // Failed to parse
	}

	// Check if token is expired or will expire within 1 minute
	expiryTime := time.Unix(claims.Exp, 0)
	return time.Now().Add(1 * time.Minute).After(expiryTime)
}

// refreshToken refreshes the access token using client credentials
func (c *APIClient) refreshToken() error {
	// Load config to get credentials
	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.CurrentContext == "" {
		return fmt.Errorf("no current context set")
	}

	// Find current context
	var currentContext *configContext.Context
	for idx := range cfg.Contexts {
		if cfg.Contexts[idx].Name == cfg.CurrentContext {
			currentContext = &cfg.Contexts[idx]
			break
		}
	}

	if currentContext == nil || currentContext.Credentials == "" {
		return fmt.Errorf("no credentials associated with current context")
	}

	// Find credential
	var credential *configContext.Credential
	for idx := range cfg.Credentials {
		if cfg.Credentials[idx].Name == currentContext.Credentials {
			credential = &cfg.Credentials[idx]
			break
		}
	}

	if credential == nil {
		return fmt.Errorf("credential '%s' not found", currentContext.Credentials)
	}

	if credential.ClientID == "" || credential.ClientSecret == "" {
		return fmt.Errorf("credential does not have client credentials for refresh")
	}

	// Find control plane
	var controlPlane *configContext.ControlPlane
	for idx := range cfg.ControlPlanes {
		if cfg.ControlPlanes[idx].Name == currentContext.ControlPlane {
			controlPlane = &cfg.ControlPlanes[idx]
			break
		}
	}

	if controlPlane == nil {
		return fmt.Errorf("control plane not found")
	}

	// Fetch token endpoint from API
	tokenEndpoint, err := getTokenEndpointFromAPI(controlPlane.URL)
	if err != nil {
		return fmt.Errorf("failed to fetch token endpoint: %w", err)
	}

	// Request new token
	authClient := &auth.ClientCredentialsAuth{
		TokenEndpoint: tokenEndpoint,
		ClientID:      credential.ClientID,
		ClientSecret:  credential.ClientSecret,
	}

	tokenResp, err := authClient.GetToken()
	if err != nil {
		return fmt.Errorf("failed to get new access token: %w", err)
	}

	// Update token in memory
	c.token = tokenResp.AccessToken

	// Update token in config
	credential.Token = tokenResp.AccessToken
	if err := config.SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save updated token: %w", err)
	}

	return nil
}

// getTokenEndpointFromAPI fetches the OIDC configuration from the API server
func getTokenEndpointFromAPI(apiURL string) (string, error) {
	req, err := http.NewRequest(
		http.MethodGet,
		apiURL+"/api/v1/oidc-config",
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add the header for OpenAPI routing
	req.Header.Set("X-Use-OpenAPI", "true")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch OIDC config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OIDC config request failed with status: %d", resp.StatusCode)
	}

	var oidcConfig struct {
		TokenEndpoint string `json:"token_endpoint"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&oidcConfig); err != nil {
		return "", fmt.Errorf("failed to decode OIDC config: %w", err)
	}

	if oidcConfig.TokenEndpoint == "" {
		return "", fmt.Errorf("token endpoint not found in OIDC config")
	}

	return oidcConfig.TokenEndpoint, nil
}

// getStoredControlPlaneConfig reads control plane config from stored configuration
func getStoredControlPlaneConfig() (*configContext.ControlPlane, error) {
	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return nil, err
	}

	if cfg.CurrentContext == "" {
		return nil, fmt.Errorf("no current context set")
	}

	// Find current context
	var currentContext *configContext.Context
	for idx := range cfg.Contexts {
		if cfg.Contexts[idx].Name == cfg.CurrentContext {
			currentContext = &cfg.Contexts[idx]
			break
		}
	}

	if currentContext == nil {
		return nil, fmt.Errorf("current context '%s' not found", cfg.CurrentContext)
	}

	// Find control plane for this context
	for idx := range cfg.ControlPlanes {
		if cfg.ControlPlanes[idx].Name == currentContext.ControlPlane {
			return &cfg.ControlPlanes[idx], nil
		}
	}

	return nil, fmt.Errorf("control plane '%s' not found", currentContext.ControlPlane)
}
