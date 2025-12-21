// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/observer/config"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	logger     *slog.Logger
	disabled   bool
}

// NewClient creates a new authz HTTP client
func NewClient(cfg *config.AuthzConfig, logger *slog.Logger) (*Client, error) {
	if !cfg.Enabled {
		logger.Info("Authorization is disabled")
		return &Client{
			disabled: true,
			logger:   logger,
		}, nil
	}

	if cfg.ServiceURL == "" {
		return nil, fmt.Errorf("authz service URL is required when authz is enabled")
	}

	if cfg.Timeout <= 0 {
		return nil, fmt.Errorf("authz timeout must be positive")
	}

	httpClient := &http.Client{
		Timeout: cfg.Timeout,
	}

	logger.Info("Authorization client initialized",
		"service_url", cfg.ServiceURL,
		"timeout", cfg.Timeout)

	return &Client{
		baseURL:    cfg.ServiceURL,
		httpClient: httpClient,
		logger:     logger,
		disabled:   false,
	}, nil
}

// Evaluate evaluates a single authorization request
func (c *Client) Evaluate(ctx context.Context, request *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	if c.disabled {
		return &authzcore.Decision{Decision: true}, nil
	}

	body, err := json.Marshal(request)
	if err != nil {
		c.logger.Error("failed to marshal evaluate request", "error", err)
		return nil, fmt.Errorf("failed to marshal evaluate request: %w", err)
	}

	url := c.baseURL + "/api/v1/authz/evaluate"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		c.logger.Error("failed to create HTTP request", "error", err)
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	// TODO: Add auth header
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("Authz service request failed", "error", err, "url", url)
		return nil, ErrAuthzServiceUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		c.logger.Warn("Authz service returned unauthorized", "status", resp.StatusCode)
		return nil, ErrAuthzUnauthorized
	}

	if resp.StatusCode == http.StatusForbidden {
		c.logger.Debug(ErrAuthzForbidden.Error())
		return &authzcore.Decision{Decision: false}, nil
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("Authz service returned error", "status", resp.StatusCode)
		return nil, fmt.Errorf("authz service returned %d", resp.StatusCode)
	}

	var decision authzcore.Decision
	if err := json.NewDecoder(resp.Body).Decode(&decision); err != nil {
		c.logger.Error("Failed to decode authz response", "error", err)
		return nil, ErrAuthzInvalidResponse
	}

	c.logger.Debug("Authorization evaluated",
		"action", request.Action,
		"resource_type", request.Resource.Type,
		"resource_id", request.Resource.ID,
		"decision", decision.Decision)

	return &decision, nil
}

// BatchEvaluate evaluates multiple authorization requests
func (c *Client) BatchEvaluate(ctx context.Context, request *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	if c.disabled {
		decisions := make([]authzcore.Decision, len(request.Requests))
		for i := range decisions {
			decisions[i] = authzcore.Decision{Decision: true}
		}
		return &authzcore.BatchEvaluateResponse{Decisions: decisions}, nil
	}

	body, err := json.Marshal(request)
	if err != nil {
		c.logger.Error("Failed to marshal batch evaluate request", "error", err)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL + "/api/v1/authz/batch-evaluate"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		c.logger.Error("Failed to create HTTP request", "error", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// TODO: Add auth header
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("Authz service batch request failed", "error", err, "url", url)
		return nil, ErrAuthzServiceUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		c.logger.Warn("Authz service returned unauthorized", "status", resp.StatusCode)
		return nil, ErrAuthzUnauthorized
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("Authz service returned error", "status", resp.StatusCode)
		return nil, fmt.Errorf("authz service returned %d", resp.StatusCode)
	}

	var response authzcore.BatchEvaluateResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		c.logger.Error("Failed to decode batch authz response", "error", err)
		return nil, ErrAuthzInvalidResponse
	}

	c.logger.Debug("Batch authorization evaluated", "request_count", len(request.Requests))

	return &response, nil
}

// GetSubjectProfile is not implemented for observer API
func (c *Client) GetSubjectProfile(ctx context.Context, request *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, errors.New("GetSubjectProfile is not supported in observer API")
}
