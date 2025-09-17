// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
)

// Client provides Prometheus query functionality
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

// QueryResponse represents a Prometheus query response
type QueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []any             `json:"value"`
		} `json:"result"`
	} `json:"data"`
	Error     string `json:"error,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
}

// MetricPoint represents a single metric data point
type MetricPoint struct {
	Metric    map[string]string `json:"metric"`
	Value     string            `json:"value"`
	Timestamp float64           `json:"timestamp"`
}

// NewClient creates a new Prometheus client
func NewClient(cfg *config.PrometheusConfig, logger *slog.Logger) *Client {
	return &Client{
		baseURL: cfg.Address,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger: logger,
	}
}

// Query executes a PromQL query
func (c *Client) Query(ctx context.Context, query string) (*QueryResponse, error) {
	c.logger.Debug("Executing Prometheus query", "query", query)

	// Build URL with query parameter
	u, err := url.Parse(fmt.Sprintf("%s/api/v1/query", c.baseURL))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	// Create request with proper URL encoding
	req, err := http.NewRequestWithContext(ctx, "POST", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add form data for the query
	form := url.Values{}
	form.Add("query", query)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Body = io.NopCloser(strings.NewReader(form.Encode()))

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse JSON response
	var queryResp QueryResponse
	if err := json.Unmarshal(body, &queryResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for Prometheus errors
	if queryResp.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: %s - %s", queryResp.ErrorType, queryResp.Error)
	}

	c.logger.Debug("Prometheus query executed successfully",
		"result_count", len(queryResp.Data.Result))

	return &queryResp, nil
}

// QueryRange executes a PromQL range query
func (c *Client) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) (*QueryResponse, error) {
	c.logger.Debug("Executing Prometheus range query",
		"query", query,
		"start", start,
		"end", end,
		"step", step)

	// Build URL
	u, err := url.Parse(fmt.Sprintf("%s/api/v1/query_range", c.baseURL))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add form data
	form := url.Values{}
	form.Add("query", query)
	form.Add("start", fmt.Sprintf("%d", start.Unix()))
	form.Add("end", fmt.Sprintf("%d", end.Unix()))
	form.Add("step", fmt.Sprintf("%.0fs", step.Seconds()))

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Body = io.NopCloser(strings.NewReader(form.Encode()))

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse JSON response
	var queryResp QueryResponse
	if err := json.Unmarshal(body, &queryResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for Prometheus errors
	if queryResp.Status != "success" {
		return nil, fmt.Errorf("prometheus range query failed: %s - %s", queryResp.ErrorType, queryResp.Error)
	}

	c.logger.Debug("Prometheus range query executed successfully",
		"result_count", len(queryResp.Data.Result))

	return &queryResp, nil
}

// HealthCheck performs a health check on Prometheus
func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	u := fmt.Sprintf("%s/-/healthy", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("prometheus health check failed with status: %d", resp.StatusCode)
	}

	return nil
}