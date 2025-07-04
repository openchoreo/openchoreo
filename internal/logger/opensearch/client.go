// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package opensearch

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"

	"github.com/openchoreo/openchoreo/internal/logger/config"
)

// Client wraps the OpenSearch client with logging and configuration
type Client struct {
	client *opensearch.Client
	config *config.OpenSearchConfig
	logger *slog.Logger
}

// NewClient creates a new OpenSearch client with the provided configuration
func NewClient(cfg *config.OpenSearchConfig, logger *slog.Logger) (*Client, error) {
	// Configure OpenSearch client
	opensearchConfig := opensearch.Config{
		Addresses: []string{cfg.Address},
		Username:  cfg.Username,
		Password:  cfg.Password,
		// TODO: Add configurable TLS settings with proper certificate verification
		// Consider adding TLSInsecureSkipVerify config option for development environments
		// while defaulting to secure certificate verification in production
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
		"indices", indices,
		"query", query)

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

// HealthCheck performs a basic health check on the OpenSearch cluster
func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.client.Info()
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	return nil
}
