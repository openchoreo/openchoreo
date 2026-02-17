// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package opensearch

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"

	"github.com/openchoreo/openchoreo/internal/observer/config"
)

const alertsIndexName = "openchoreo-alerts"

// Client wraps the OpenSearch client with logging and configuration
type Client struct {
	client *opensearch.Client
	config *config.OpenSearchConfig
	logger *slog.Logger
}

// NewClient creates a new OpenSearch client with the provided configuration
func NewClient(cfg *config.OpenSearchConfig, logger *slog.Logger) (*Client, error) {
	opensearchConfig := opensearch.Config{
		Addresses: []string{cfg.Address},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // G402: Using self-signed cert
			},
		},
		Username: cfg.Username,
		Password: cfg.Password,
	}

	client, err := opensearch.NewClient(opensearchConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenSearch client: %w", err)
	}

	// Test connection
	info, err := client.Info()
	if err != nil {
		logger.Warn("Failed to connect to OpenSearch", "error", err)
	} else {
		logger.Info("Connected to OpenSearch", "status", info.Status())
	}

	return &Client{
		client: client,
		config: cfg,
		logger: logger,
	}, nil
}

// Search executes a search request against OpenSearch
func (c *Client) Search(ctx context.Context, indices []string, query map[string]interface{}) (*SearchResponse, error) {
	c.logger.Debug("Executing search",
		"indices", indices)

	if c.logger.Enabled(ctx, slog.LevelDebug) {
		queryJSON, err := json.MarshalIndent(query, "", "  ")
		if err == nil {
			fmt.Println("OpenSearch Query:")
			fmt.Println(string(queryJSON))
		}
	}

	req := opensearchapi.SearchRequest{
		Index:             indices,
		Body:              buildSearchBody(query),
		IgnoreUnavailable: opensearchapi.BoolPtr(true),
	}

	res, err := req.Do(ctx, c.client)
	if err != nil {
		c.logger.Error("Search request failed", "error", err)
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		c.logger.Error("Search request returned error",
			"status", res.Status(),
			"error", res.String())
		return nil, fmt.Errorf("search request failed with status: %s", res.Status())
	}

	response, err := parseSearchResponse(res.Body)
	if err != nil {
		c.logger.Error("Failed to parse search response", "error", err)
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	c.logger.Debug("Search completed",
		"total_hits", response.Hits.Total.Value,
		"returned_hits", len(response.Hits.Hits))

	return response, nil
}

// GetIndexMapping retrieves the mapping for a specific index
func (c *Client) GetIndexMapping(ctx context.Context, index string) (*MappingResponse, error) {
	req := opensearchapi.IndicesGetMappingRequest{
		Index: []string{index},
	}

	res, err := req.Do(ctx, c.client)
	if err != nil {
		c.logger.Error("Get mapping request failed", "error", err)
		return nil, fmt.Errorf("get mapping request failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		c.logger.Error("Get mapping request returned error",
			"status", res.Status(),
			"error", res.String())
		return nil, fmt.Errorf("get mapping request failed with status: %s", res.Status())
	}

	mapping, err := parseMappingResponse(res.Body)
	if err != nil {
		c.logger.Error("Failed to parse mapping response", "error", err)
		return nil, fmt.Errorf("failed to parse mapping response: %w", err)
	}

	return mapping, nil
}

// SearchMonitorByName searches alerting monitors by name using the Alerting plugin API.
func (c *Client) SearchMonitorByName(ctx context.Context, name string) (string, bool, error) {
	path := "/_plugins/_alerting/monitors/_search"
	queryBody := fmt.Sprintf(`{
		"query": {
				"match_phrase": {
						"monitor.name": "%s"
				}
		}
  }`, name)

	req, err := http.NewRequest("POST", path, strings.NewReader(queryBody))
	if err != nil {
		return "", false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := c.client.Perform(req)
	if err != nil {
		return "", false, fmt.Errorf("monitor search request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("monitor search request failed with status: %d", res.StatusCode)
	}

	// parse the monitor search response using parseSearchResponse function
	parsed, err := parseSearchResponse(res.Body)
	if err != nil {
		return "", false, fmt.Errorf("failed to parse monitor search response: %w", err)
	}

	if parsed.Hits.Total.Value == 0 || len(parsed.Hits.Hits) == 0 {
		return "", false, nil
	}
	if parsed.Hits.Hits[0].ID == "" {
		return "", false, fmt.Errorf("monitor search response missing _id field")
	}
	return parsed.Hits.Hits[0].ID, true, nil
}

// CreateMonitor creates a new alerting monitor using the Alerting plugin API.
func (c *Client) CreateMonitor(ctx context.Context, monitor map[string]interface{}) (string, int64, error) {
	body, err := json.Marshal(monitor)
	if err != nil {
		return "", 0, fmt.Errorf("failed to marshal monitor: %w", err)
	}
	c.logger.Debug("Creating monitor", "body", string(body))

	path := "/_plugins/_alerting/monitors"
	req, err := http.NewRequest("POST", path, bytes.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := c.client.Perform(req)
	if err != nil {
		return "", 0, fmt.Errorf("monitor create request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(res.Body)
		c.logger.Error("Monitor create failed",
			"status", res.StatusCode,
			"response", string(bodyBytes))
		return "", 0, fmt.Errorf("monitor create request failed with status: %d, response: %s", res.StatusCode, string(bodyBytes))
	}

	type MonitorUpsertResponse struct {
		LastUpdateTime int64 `json:"last_update_time"`
	}
	var parsed struct {
		ID      string                `json:"_id"`
		Monitor MonitorUpsertResponse `json:"monitor"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return "", 0, fmt.Errorf("failed to parse monitor create response: %w", err)
	}

	c.logger.Debug("Monitor create response: ", slog.Int64("last_update_time", parsed.Monitor.LastUpdateTime), "id", slog.String("id", parsed.ID))
	return parsed.ID, parsed.Monitor.LastUpdateTime, nil
}

// GetMonitorByID retrieves an alerting monitor by ID using the Alerting plugin API.
func (c *Client) GetMonitorByID(ctx context.Context, monitorID string) (map[string]interface{}, error) {
	path := fmt.Sprintf("/_plugins/_alerting/monitors/%s", monitorID)
	req, err := http.NewRequestWithContext(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := c.client.Perform(req)
	if err != nil {
		return nil, fmt.Errorf("monitor get request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(res.Body)
		c.logger.Error("Monitor get failed",
			"status", res.StatusCode,
			"monitor_id", monitorID,
			"response", string(bodyBytes))
		return nil, fmt.Errorf("monitor get request failed with status: %d, response: %s", res.StatusCode, string(bodyBytes))
	}

	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse monitor get response: %w", err)
	}

	// Extract the monitor object from the response
	if monitor, ok := response["monitor"].(map[string]interface{}); ok {
		return monitor, nil
	}

	return nil, fmt.Errorf("monitor object not found in response")
}

// UpdateMonitor updates an existing alerting monitor using the Alerting plugin API.
func (c *Client) UpdateMonitor(ctx context.Context, monitorID string, monitor map[string]interface{}) (int64, error) {
	body, err := json.Marshal(monitor)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal monitor: %w", err)
	}
	c.logger.Debug("Updating monitor", "monitor_id", monitorID, "body", string(body))

	path := fmt.Sprintf("/_plugins/_alerting/monitors/%s", monitorID)
	req, err := http.NewRequestWithContext(ctx, "PUT", path, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := c.client.Perform(req)
	if err != nil {
		return 0, fmt.Errorf("monitor update request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(res.Body)
		c.logger.Error("Monitor update failed",
			"status", res.StatusCode,
			"monitor_id", monitorID,
			"response", string(bodyBytes))
		return 0, fmt.Errorf("monitor update request failed with status: %d, response: %s", res.StatusCode, string(bodyBytes))
	}

	type MonitorUpsertResponse struct {
		LastUpdateTime int64 `json:"last_update_time"`
	}
	var parsed struct {
		ID      string                `json:"_id"`
		Monitor MonitorUpsertResponse `json:"monitor"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return 0, fmt.Errorf("failed to parse monitor update response: %w", err)
	}

	c.logger.Debug("Monitor updated successfully",
		"monitor_id", monitorID,
		"last_update_time", parsed.Monitor.LastUpdateTime)
	return parsed.Monitor.LastUpdateTime, nil
}

// DeleteMonitor deletes an alerting monitor using the Alerting plugin API.
func (c *Client) DeleteMonitor(ctx context.Context, monitorID string) error {
	path := fmt.Sprintf("/_plugins/_alerting/monitors/%s", monitorID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", path, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := c.client.Perform(req)
	if err != nil {
		return fmt.Errorf("monitor delete request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(res.Body)
		c.logger.Error("Monitor delete failed",
			"status", res.StatusCode,
			"monitor_id", monitorID,
			"response", string(bodyBytes))
		return fmt.Errorf("monitor delete request failed with status: %d, response: %s", res.StatusCode, string(bodyBytes))
	}

	c.logger.Debug("Monitor deleted successfully", "monitor_id", monitorID)
	return nil
}

// WriteAlertEntry writes an alert entry to OpenSearch (openchoreo-alerts index)
func (c *Client) WriteAlertEntry(ctx context.Context, entry map[string]interface{}) (string, error) {
	body, err := json.Marshal(entry)
	if err != nil {
		return "", fmt.Errorf("failed to marshal alert entry: %w", err)
	}

	req := opensearchapi.IndexRequest{
		Index:   alertsIndexName,
		Body:    bytes.NewReader(body),
		Refresh: "true",
	}

	res, err := req.Do(ctx, c.client)
	if err != nil {
		return "", fmt.Errorf("alert index request failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		c.logger.Error("Alert index request returned error",
			"status", res.Status(),
			"response", string(bodyBytes))
		return "", fmt.Errorf("alert index request failed with status: %s, response: %s", res.Status(), string(bodyBytes))
	}

	var parsed struct {
		ID string `json:"_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("failed to parse alert index response: %w", err)
	}

	c.logger.Debug("Alert entry written", "alert_id", parsed.ID)
	return parsed.ID, nil
}
