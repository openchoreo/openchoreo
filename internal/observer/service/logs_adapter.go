// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/api/logsadapterclientgen"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

type LogsAdapter struct {
	baseURL       string
	httpClient    *http.Client
	adapterClient *logsadapterclientgen.Client
}

type LogsAdapterConfig struct {
	BaseURL string
	Timeout time.Duration
}

// componentSearchScope represents the search scope for component log queries
// matching the adapter's expected request format.
type componentSearchScope struct {
	Namespace      string `json:"namespace"`
	ProjectUID     string `json:"projectUid,omitempty"`
	ComponentUID   string `json:"componentUid,omitempty"`
	EnvironmentUID string `json:"environmentUid,omitempty"`
}

// workflowSearchScope represents the search scope for workflow log queries
// matching the adapter's expected request format.
type workflowSearchScope struct {
	Namespace       string `json:"namespace"`
	WorkflowRunName string `json:"workflowRunName,omitempty"`
}

// backendLogsRequest represents the request body format expected by the logs adapter.
type backendLogsRequest struct {
	SearchScope  any       `json:"searchScope"`
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
	Limit        int       `json:"limit,omitempty"`
	SortOrder    string    `json:"sortOrder,omitempty"`
	SearchPhrase string    `json:"searchPhrase,omitempty"`
	LogLevels    []string  `json:"logLevels,omitempty"`
}

// backendComponentLogsResponse matches the adapter's JSON response format for component logs.
type backendComponentLogsResponse struct {
	Logs   []backendComponentLogEntry `json:"logs"`
	Total  int                        `json:"total"`
	TookMs int                        `json:"tookMs"`
}

type backendComponentLogEntry struct {
	Timestamp time.Time                   `json:"timestamp"`
	Log       string                      `json:"log"`
	Level     string                      `json:"level"`
	Metadata  backendComponentLogMetadata `json:"metadata"`
}

type backendComponentLogMetadata struct {
	ComponentUID    string `json:"componentUid"`
	ComponentName   string `json:"componentName"`
	EnvironmentUID  string `json:"environmentUid"`
	EnvironmentName string `json:"environmentName"`
	ProjectUID      string `json:"projectUid"`
	ProjectName     string `json:"projectName"`
	NamespaceName   string `json:"namespaceName"`
	PodName         string `json:"podName"`
	PodNamespace    string `json:"podNamespace"`
	ContainerName   string `json:"containerName"`
}

// backendWorkflowLogsResponse matches the adapter's JSON response format for workflow logs.
type backendWorkflowLogsResponse struct {
	Logs   []backendWorkflowLogEntry `json:"logs"`
	Total  int                       `json:"total"`
	TookMs int                       `json:"tookMs"`
}

type backendWorkflowLogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Log       string                 `json:"log"`
	Metadata  map[string]interface{} `json:"metadata"`
}

func NewLogsAdapter(config LogsAdapterConfig) (*LogsAdapter, error) {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	httpClient := &http.Client{
		Timeout: config.Timeout,
	}

	adapterClient, err := logsadapterclientgen.NewClient(config.BaseURL, logsadapterclientgen.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter client: %w", err)
	}

	return &LogsAdapter{
		baseURL:       config.BaseURL,
		httpClient:    httpClient,
		adapterClient: adapterClient,
	}, nil
}

// GetComponentApplicationLogs implements observability.LogsAdapter interface
// It makes an HTTP POST request to the logs API with component application logs parameters
func (p *LogsAdapter) GetComponentApplicationLogs(ctx context.Context, params observability.ComponentApplicationLogsParams) (*observability.ComponentApplicationLogsResult, error) {
	url := fmt.Sprintf("%s/api/v1/logs/query", p.baseURL)

	adapterReq := backendLogsRequest{
		SearchScope: componentSearchScope{
			Namespace:      params.Namespace,
			ProjectUID:     params.ProjectID,
			ComponentUID:   params.ComponentID,
			EnvironmentUID: params.EnvironmentID,
		},
		StartTime:    params.StartTime,
		EndTime:      params.EndTime,
		Limit:        params.Limit,
		SortOrder:    params.SortOrder,
		SearchPhrase: params.SearchPhrase,
		LogLevels:    params.LogLevels,
	}

	requestBody, err := json.Marshal(adapterReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request for component logs to logs adapter: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var adapterResp backendComponentLogsResponse
	if err := json.NewDecoder(resp.Body).Decode(&adapterResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	logs := make([]observability.LogEntry, 0, len(adapterResp.Logs))
	for _, l := range adapterResp.Logs {
		logs = append(logs, observability.LogEntry{
			Timestamp:       l.Timestamp,
			Log:             l.Log,
			LogLevel:        l.Level,
			ComponentID:     l.Metadata.ComponentUID,
			ComponentName:   l.Metadata.ComponentName,
			EnvironmentID:   l.Metadata.EnvironmentUID,
			EnvironmentName: l.Metadata.EnvironmentName,
			ProjectID:       l.Metadata.ProjectUID,
			ProjectName:     l.Metadata.ProjectName,
			Namespace:       l.Metadata.NamespaceName,
			NamespaceName:   l.Metadata.NamespaceName,
			PodName:         l.Metadata.PodName,
			PodNamespace:    l.Metadata.PodNamespace,
			ContainerName:   l.Metadata.ContainerName,
		})
	}

	return &observability.ComponentApplicationLogsResult{
		Logs:       logs,
		TotalCount: adapterResp.Total,
		Took:       adapterResp.TookMs,
	}, nil
}

// GetWorkflowLogs implements observability.LogsAdapter interface
// It makes an HTTP POST request to the logs API with workflow logs parameters
func (p *LogsAdapter) GetWorkflowLogs(ctx context.Context, params observability.WorkflowLogsParams) (*observability.WorkflowLogsResult, error) {
	url := fmt.Sprintf("%s/api/v1/logs/query", p.baseURL)

	adapterReq := backendLogsRequest{
		SearchScope: workflowSearchScope{
			Namespace:       params.Namespace,
			WorkflowRunName: params.WorkflowRunName,
		},
		StartTime:    params.StartTime,
		EndTime:      params.EndTime,
		Limit:        params.Limit,
		SortOrder:    params.SortOrder,
		SearchPhrase: params.SearchPhrase,
		LogLevels:    params.LogLevels,
	}

	requestBody, err := json.Marshal(adapterReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request for workflow logs to logs adapter: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var adapterResp backendWorkflowLogsResponse
	if err := json.NewDecoder(resp.Body).Decode(&adapterResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	logs := make([]observability.WorkflowLogEntry, 0, len(adapterResp.Logs))
	for _, l := range adapterResp.Logs {
		entry := observability.WorkflowLogEntry{
			Timestamp: l.Timestamp,
			Log:       l.Log,
		}
		if podName, ok := l.Metadata["podName"].(string); ok {
			entry.PodName = podName
		}
		if podNamespace, ok := l.Metadata["podNamespace"].(string); ok {
			entry.PodNamespace = podNamespace
		}
		if containerName, ok := l.Metadata["containerName"].(string); ok {
			entry.ContainerName = containerName
		}
		logs = append(logs, entry)
	}

	return &observability.WorkflowLogsResult{
		Logs:       logs,
		TotalCount: adapterResp.Total,
		Took:       adapterResp.TookMs,
	}, nil
}
