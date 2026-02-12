// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ObserverClient provides HTTP client for Observer API
type ObserverClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// LogEntry represents a single log entry from observer
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Log       string `json:"log"`
	Level     string `json:"level,omitempty"`
	Stream    string `json:"stream,omitempty"`
}

// LogResponse represents the response from the observer logs API
type LogResponse struct {
	Logs       []LogEntry `json:"logs"`
	TotalCount int        `json:"totalCount"`
	TookMs     int        `json:"tookMs"`
}

// ComponentLogsRequest represents the request body for component logs API
type ComponentLogsRequest struct {
	StartTime       string   `json:"startTime"`
	EndTime         string   `json:"endTime"`
	EnvironmentID   string   `json:"environmentId"`
	ComponentName   string   `json:"componentName"`
	ProjectName     string   `json:"projectName"`
	NamespaceName   string   `json:"namespaceName"`
	EnvironmentName string   `json:"environmentName"`
	Limit           int64    `json:"limit"`
	SortOrder       string   `json:"sortOrder"`
	LogType         string   `json:"logType"`
	SearchPhrase    string   `json:"searchPhrase,omitempty"`
	LogLevels       []string `json:"logLevels,omitempty"`
}

// NewObserverClient creates a new Observer API client
func NewObserverClient(observerURL, token string) *ObserverClient {
	return &ObserverClient{
		baseURL:    observerURL,
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchComponentLogs fetches logs for a component from the observer API
func (c *ObserverClient) FetchComponentLogs(ctx context.Context, componentID string, req ComponentLogsRequest) (*LogResponse, error) {
	path := fmt.Sprintf("/api/logs/component/%s", componentID)

	resp, err := c.doRequest(ctx, "POST", path, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("observer API returned status %d", resp.StatusCode)
	}

	var logResponse LogResponse
	if err := json.Unmarshal(body, &logResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &logResponse, nil
}

// doRequest performs HTTP request with proper headers
func (c *ObserverClient) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	// Reuse legacy_client.go's APIClient doRequest logic
	legacyClient := &APIClient{
		baseURL:    c.baseURL,
		token:      c.token,
		httpClient: c.httpClient,
	}

	return legacyClient.doRequest(ctx, method, path, body)
}
