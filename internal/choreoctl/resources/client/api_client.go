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
	"net/url"
	"time"

	"github.com/openchoreo/openchoreo/internal/choreoctl/cmd/config"
	configContext "github.com/openchoreo/openchoreo/pkg/cli/cmd/config"
	"github.com/openchoreo/openchoreo/pkg/constants"
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

// ResponseMetadata contains metadata for list responses
type ResponseMetadata struct {
	ResourceVersion string `json:"resourceVersion"`
	Continue        string `json:"continue,omitempty"`
	HasMore         bool   `json:"hasMore"`
}

// ListResponse represents a list response with items and metadata
type ListResponse struct {
	Items    []OrganizationResponse `json:"items"`
	Metadata ResponseMetadata       `json:"metadata"`
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
		Items    []ProjectResponse `json:"items"`
		Metadata ResponseMetadata  `json:"metadata"`
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
		Items    []ComponentResponse `json:"items"`
		Metadata ResponseMetadata    `json:"metadata"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

/*
// TO ENABLE OPENCHOREO REST API CLIENT:
// Uncomment the following structs and methods to enable the REST API client for single resource retrieval.
// This will allow choreoctl to use the REST API instead of the Kubernetes client directly.

// GetOrganizationResponse represents the response for a single organization
type GetOrganizationResponse struct {
	Success bool                 `json:"success"`
	Data    OrganizationResponse `json:"data"`
	Error   string               `json:"error,omitempty"`
	Code    string               `json:"code,omitempty"`
}

// GetProjectResponse represents the response for a single project
type GetProjectResponse struct {
	Success bool            `json:"success"`
	Data    ProjectResponse `json:"data"`
	Error   string          `json:"error,omitempty"`
	Code    string          `json:"code,omitempty"`
}

// GetComponentResponse represents the response for a single component
type GetComponentResponse struct {
	Success bool              `json:"success"`
	Data    ComponentResponse `json:"data"`
	Error   string            `json:"error,omitempty"`
	Code    string            `json:"code,omitempty"`
}
*/

// NewAPIClient creates a new API client with control plane auto-detection
func NewAPIClient() (*APIClient, error) {
	cfg, err := getStoredControlPlaneConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to detect control plane: %w", err)
	}

	return &APIClient{
		baseURL:    cfg.Endpoint,
		token:      cfg.Token,
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
func (c *APIClient) ListOrganizations(ctx context.Context, maxItems int) ([]OrganizationResponse, error) {
	var allOrganizations []OrganizationResponse
	continueToken := ""
	pageLimit := constants.DefaultPageLimit
	if maxItems > 0 {
		// Cap at MaxPageLimit (better to make fewer, larger requests)
		if maxItems > constants.MaxPageLimit {
			pageLimit = constants.MaxPageLimit
		} else {
			pageLimit = maxItems
		}
	}

	for {
		params := url.Values{}
		effectiveLimit := pageLimit
		if maxItems > 0 {
			remaining := maxItems - len(allOrganizations)
			if remaining <= 0 {
				break
			}
			if remaining < effectiveLimit {
				effectiveLimit = remaining
			}
		}
		params.Set("limit", fmt.Sprintf("%d", effectiveLimit))
		if continueToken != "" {
			params.Set("continue", continueToken)
		}

		resp, err := c.getWithParams(ctx, "/api/v1/orgs", params)
		if err != nil {
			return nil, fmt.Errorf("failed to make list organizations request: %w", err)
		}

		// Handle HTTP 410 Gone (expired continue token)
		if resp.StatusCode == http.StatusGone {
			resp.Body.Close()
			continueToken = ""
			allOrganizations = nil
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
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

		allOrganizations = append(allOrganizations, listResp.Data.Items...)

		if maxItems > 0 && len(allOrganizations) >= maxItems {
			allOrganizations = allOrganizations[:maxItems]
			break
		}

		// Check if there are more pages
		if !listResp.Data.Metadata.HasMore || listResp.Data.Metadata.Continue == "" {
			break
		}
		continueToken = listResp.Data.Metadata.Continue
	}

	return allOrganizations, nil
}

// ListProjects retrieves all projects for an organization from the API
func (c *APIClient) ListProjects(ctx context.Context, orgName string, maxItems int) ([]ProjectResponse, error) {
	var allProjects []ProjectResponse
	continueToken := ""
	basePath := fmt.Sprintf("/api/v1/orgs/%s/projects", orgName)
	pageLimit := constants.DefaultPageLimit
	if maxItems > 0 {
		// Cap at MaxPageLimit (better to make fewer, larger requests)
		if maxItems > constants.MaxPageLimit {
			pageLimit = constants.MaxPageLimit
		} else {
			pageLimit = maxItems
		}
	}

	for {
		params := url.Values{}
		effectiveLimit := pageLimit
		if maxItems > 0 {
			remaining := maxItems - len(allProjects)
			if remaining <= 0 {
				break
			}
			if remaining < effectiveLimit {
				effectiveLimit = remaining
			}
		}
		params.Set("limit", fmt.Sprintf("%d", effectiveLimit))
		if continueToken != "" {
			params.Set("continue", continueToken)
		}

		resp, err := c.getWithParams(ctx, basePath, params)
		if err != nil {
			return nil, fmt.Errorf("failed to make list projects request: %w", err)
		}

		// Handle HTTP 410 Gone (expired continue token)
		if resp.StatusCode == http.StatusGone {
			resp.Body.Close()
			continueToken = ""
			allProjects = nil
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
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

		allProjects = append(allProjects, listResp.Data.Items...)

		if maxItems > 0 && len(allProjects) >= maxItems {
			allProjects = allProjects[:maxItems]
			break
		}

		// Check if there are more pages
		if !listResp.Data.Metadata.HasMore || listResp.Data.Metadata.Continue == "" {
			break
		}
		continueToken = listResp.Data.Metadata.Continue
	}

	return allProjects, nil
}

// ListComponents retrieves all components for an organization and project from the API
func (c *APIClient) ListComponents(ctx context.Context, orgName, projectName string, maxItems int) ([]ComponentResponse, error) {
	var allComponents []ComponentResponse
	continueToken := ""
	basePath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/components", orgName, projectName)
	pageLimit := constants.DefaultPageLimit
	if maxItems > 0 {
		// Cap at MaxPageLimit (better to make fewer, larger requests)
		if maxItems > constants.MaxPageLimit {
			pageLimit = constants.MaxPageLimit
		} else {
			pageLimit = maxItems
		}
	}

	for {
		params := url.Values{}
		effectiveLimit := pageLimit
		if maxItems > 0 {
			remaining := maxItems - len(allComponents)
			if remaining <= 0 {
				break
			}
			if remaining < effectiveLimit {
				effectiveLimit = remaining
			}
		}
		params.Set("limit", fmt.Sprintf("%d", effectiveLimit))
		if continueToken != "" {
			params.Set("continue", continueToken)
		}

		resp, err := c.getWithParams(ctx, basePath, params)
		if err != nil {
			return nil, fmt.Errorf("failed to make list components request: %w", err)
		}

		// Handle HTTP 410 Gone (expired continue token)
		if resp.StatusCode == http.StatusGone {
			resp.Body.Close()
			continueToken = ""
			allComponents = nil
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
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

		allComponents = append(allComponents, listResp.Data.Items...)

		if maxItems > 0 && len(allComponents) >= maxItems {
			allComponents = allComponents[:maxItems]
			break
		}

		// Check if there are more pages
		if !listResp.Data.Metadata.HasMore || listResp.Data.Metadata.Continue == "" {
			break
		}
		continueToken = listResp.Data.Metadata.Continue
	}

	return allComponents, nil
}

/*
// TO ENABLE OPENCHOREO REST API CLIENT:
// Uncomment the following structs and methods to enable the REST API client for single resource retrieval.
// This will allow choreoctl to use the REST API instead of the Kubernetes client directly.

// GetOrganization retrieves a single organization by name
func (c *APIClient) GetOrganization(ctx context.Context, name string) (*OrganizationResponse, error) {
	path := fmt.Sprintf("/api/v1/orgs/%s", name)
	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to make get organization request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var getResp GetOrganizationResponse
	if err := json.Unmarshal(body, &getResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !getResp.Success {
		return nil, fmt.Errorf("get organization failed: %s", getResp.Error)
	}

	return &getResp.Data, nil
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

// GetProject retrieves a single project by org and project name
func (c *APIClient) GetProject(ctx context.Context, orgName, projectName string) (*ProjectResponse, error) {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s", orgName, projectName)
	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to make get project request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var getResp GetProjectResponse
	if err := json.Unmarshal(body, &getResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !getResp.Success {
		return nil, fmt.Errorf("get project failed: %s", getResp.Error)
	}

	return &getResp.Data, nil
}

// GetComponent retrieves a single component by org, project, and component name
func (c *APIClient) GetComponent(ctx context.Context, orgName, projectName, componentName string) (*ComponentResponse, error) {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/components/%s", orgName, projectName, componentName)
	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to make get component request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var getResp GetComponentResponse
	if err := json.Unmarshal(body, &getResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !getResp.Success {
		return nil, fmt.Errorf("get component failed: %s", getResp.Error)
	}

	return &getResp.Data, nil
}

// HTTP helper methods
func (c *APIClient) get(ctx context.Context, path string) (*http.Response, error) {
	return c.doRequest(ctx, "GET", path, nil)
}

// getWithParams performs a GET request with query parameters
func (c *APIClient) getWithParams(ctx context.Context, path string, params url.Values) (*http.Response, error) {
	if len(params) > 0 {
		// Parse the path to safely add query parameters
		parsedURL, err := url.Parse(path)
		if err != nil {
			return nil, fmt.Errorf("failed to parse path: %w", err)
		}

		// Merge existing query parameters with new ones
		q := parsedURL.Query()
		for key, values := range params {
			for _, value := range values {
				q.Add(key, value)
			}
		}
		parsedURL.RawQuery = q.Encode()
		path = parsedURL.String()
	}
	return c.doRequest(ctx, "GET", path, nil)
}

func (c *APIClient) post(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	return c.doRequest(ctx, "POST", path, body)
}

func (c *APIClient) delete(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	return c.doRequest(ctx, "DELETE", path, body)
}

func (c *APIClient) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
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

// getStoredControlPlaneConfig reads control plane config from stored configuration
func getStoredControlPlaneConfig() (*configContext.ControlPlane, error) {
	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return nil, err
	}

	if cfg.ControlPlane == nil {
		return nil, fmt.Errorf("no control plane configured")
	}

	return cfg.ControlPlane, nil
}
