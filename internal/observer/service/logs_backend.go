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

	"github.com/openchoreo/openchoreo/pkg/observability"
)

type LogsBackend struct {
	baseURL    string
	httpClient *http.Client
}

type LogsBackendConfig struct {
	BaseURL string
	Timeout time.Duration
}

func NewLogsBackend(config LogsBackendConfig) *LogsBackend {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &LogsBackend{
		baseURL: config.BaseURL,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// GetComponentApplicationLogs implements observability.LogsBackend interface
// It makes an HTTP POST request to the logs API with component application logs parameters
func (p *LogsBackend) GetComponentApplicationLogs(ctx context.Context, params observability.ComponentApplicationLogsParams) (*observability.ComponentApplicationLogsResult, error) {
	url := fmt.Sprintf("%s/api/v1/component-application-logs", p.baseURL)

	requestBody, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("Failed to create HTTP request for component logs to logs backend: %w", err)
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

	var result observability.ComponentApplicationLogsResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}
