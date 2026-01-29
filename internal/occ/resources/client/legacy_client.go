// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
)

// TODO: This client will be replaced by OpenAPI generated client

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

// NamespaceResponse represents an namespace from the API
type NamespaceResponse struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
}

// ListResponse represents a paginated list response
type ListResponse struct {
	Items      []NamespaceResponse `json:"items"`
	TotalCount int                 `json:"totalCount"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"pageSize"`
}

// ListNamespacesResponse represents the response from listing namespaces
type ListNamespacesResponse struct {
	Success bool         `json:"success"`
	Data    ListResponse `json:"data"`
	Error   string       `json:"error,omitempty"`
	Code    string       `json:"code,omitempty"`
}

// ProjectResponse represents a project from the API
type ProjectResponse struct {
	Name               string `json:"name"`
	NamespaceName      string `json:"namespaceName"`
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
	Name          string `json:"name"`
	NamespaceName string `json:"namespaceName"`
	ProjectName   string `json:"projectName"`
	DisplayName   string `json:"displayName,omitempty"`
	Description   string `json:"description,omitempty"`
	Type          string `json:"type"`
	CreatedAt     string `json:"createdAt"`
	Status        string `json:"status,omitempty"`
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
	// Get control plane
	controlPlane, err := config.GetCurrentControlPlane()
	if err != nil {
		return nil, fmt.Errorf("failed to get control plane: %w", err)
	}

	// Get credential and token (may not exist yet if not logged in)
	token := ""
	credential, err := config.GetCurrentCredential()
	if err == nil && credential != nil {
		token = credential.Token
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

// ListNamespaces retrieves all namespaces from the API
func (c *APIClient) ListNamespaces(ctx context.Context) ([]NamespaceResponse, error) {
	resp, err := c.get(ctx, "/api/v1/orgs")
	if err != nil {
		return nil, fmt.Errorf("failed to make list namespaces request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var listResp ListNamespacesResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !listResp.Success {
		return nil, fmt.Errorf("list namespaces failed: %s", listResp.Error)
	}

	return listResp.Data.Items, nil
}

// ListProjects retrieves all projects for an namespace from the API
func (c *APIClient) ListProjects(ctx context.Context, namespaceName string) ([]ProjectResponse, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/projects", namespaceName)
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

// ListComponents retrieves all components for an namespace and project from the API
func (c *APIClient) ListComponents(ctx context.Context, namespaceName, projectName string) ([]ComponentResponse, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/projects/%s/components", namespaceName, projectName)
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
func (c *APIClient) GetComponentTypeSchema(ctx context.Context, namespaceName, ctName string) (*json.RawMessage, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/component-types/%s/schema", namespaceName, ctName)
	return c.getSchema(ctx, path)
}

// GetTraitSchema fetches Trait schema from the API
func (c *APIClient) GetTraitSchema(ctx context.Context, namespaceName, traitName string) (*json.RawMessage, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/traits/%s/schema", namespaceName, traitName)
	return c.getSchema(ctx, path)
}

// GetComponentWorkflowSchema fetches ComponentWorkflow schema from the API
func (c *APIClient) GetComponentWorkflowSchema(ctx context.Context, namespaceName, cwName string) (*json.RawMessage, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/component-workflows/%s/schema", namespaceName, cwName)
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
	if c.token != "" && auth.IsTokenExpired(c.token) {
		newToken, err := auth.RefreshToken()
		if err != nil {
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
		c.token = newToken
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
