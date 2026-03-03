// Copyright 2026 The OpenChoreo Authors
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

type TracingAdapter struct {
	baseURL    string
	httpClient *http.Client
}

type TracingAdapterConfig struct {
	BaseURL string
	Timeout time.Duration
}

func NewTracingAdapter(config TracingAdapterConfig) *TracingAdapter {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &TracingAdapter{
		baseURL: config.BaseURL,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// GetTraces implements observability.TracingAdapter interface
func (t *TracingAdapter) GetTraces(ctx context.Context, params observability.TracesQueryParams) (*observability.TracesQueryResult, error) {
	url := fmt.Sprintf("%s/api/v1/traces", t.baseURL)

	requestBody, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request for traces to adapter: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result observability.TracesQueryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}
