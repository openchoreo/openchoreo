// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
)

// authz evaluate endpoint
const evaluatesEndpoint = "/api/v1/authz/evaluates"

type Client struct {
	httpClient *http.Client
	baseURL    string
	logger     *slog.Logger
}

// NewClient creates a new authz HTTP client
func NewClient(cfg *config.AuthzConfig, logger *slog.Logger) (*Client, error) {
	if cfg.ServiceURL == "" {
		return nil, fmt.Errorf("authz service URL is required")
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
	}, nil
}

// Evaluate evaluates a single authorization request via the unified evaluates endpoint.
func (c *Client) Evaluate(ctx context.Context, request *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	if request == nil {
		return nil, fmt.Errorf("evaluate request must not be nil")
	}

	decisions, err := c.evaluate(ctx, []authzcore.EvaluateRequest{*request})
	if err != nil {
		return nil, err
	}

	if len(decisions) == 0 {
		c.logger.Error("Authz service returned empty decisions array")
		return nil, ErrAuthzInvalidResponse
	}

	decision := decisions[0]

	c.logger.Debug("Authorization evaluated",
		"action", request.Action,
		"resource_type", request.Resource.Type,
		"resource_id", request.Resource.ID,
		"decision", decision.Decision,
		"reason", decision.Context)

	return &decision, nil
}

// BatchEvaluate evaluates multiple authorization requests via the unified evaluates endpoint.
func (c *Client) BatchEvaluate(ctx context.Context, request *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("batch evaluate request must not be nil")
	}

	decisions, err := c.evaluate(ctx, request.Requests)
	if err != nil {
		return nil, err
	}

	if len(decisions) != len(request.Requests) {
		c.logger.Error("Decisions count mismatch", "expected", len(request.Requests), "got", len(decisions))
		return nil, ErrAuthzInvalidResponse
	}

	c.logger.Debug("Batch authorization evaluated", "request_count", len(request.Requests))

	return &authzcore.BatchEvaluateResponse{Decisions: decisions}, nil
}

// GetSubjectProfile is not implemented for observer API
func (c *Client) GetSubjectProfile(ctx context.Context, request *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, errors.New("GetSubjectProfile is not supported in observer API")
}

// evaluate sends the authorization requests to the authz service and returns the decisions
func (c *Client) evaluate(ctx context.Context, requests []authzcore.EvaluateRequest) ([]authzcore.Decision, error) {
	body, err := json.Marshal(requests)
	if err != nil {
		c.logger.Error("Failed to marshal evaluate request", "error", err)
		return nil, fmt.Errorf("failed to marshal evaluate request: %w", err)
	}

	c.logger.Debug("Authz Request Body", "json", string(body))

	url := c.baseURL + evaluatesEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		c.logger.Error("Failed to create HTTP request", "error", err)
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Extract and forward the authentication token from the incoming request context
	if token := jwt.GetTokenFromContext(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("Authz service request failed", "error", err, "url", url)
		return nil, ErrAuthzServiceUnavailable
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)

	if err != nil {
		c.logger.Error("Failed to read authz response body", "error", err)
		return nil, ErrAuthzInvalidResponse
	}

	if resp.StatusCode == http.StatusUnauthorized {
		c.logger.Warn("Authz service returned unauthorized", "status", resp.StatusCode)
		return nil, ErrAuthzUnauthorized
	}

	if resp.StatusCode == http.StatusForbidden {
		c.logger.Debug("Authz service returned forbidden", "status", resp.StatusCode, "response_body", string(bodyBytes))
		return nil, ErrAuthzForbidden
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("Authz service returned error", "status", resp.StatusCode, "response_body", string(bodyBytes))
		return nil, fmt.Errorf("authz service returned %d", resp.StatusCode)
	}

	var decisions []authzcore.Decision
	if err := json.Unmarshal(bodyBytes, &decisions); err != nil {
		c.logger.Error("Failed to decode authz response", "error", err, "body", string(bodyBytes))
		return nil, ErrAuthzInvalidResponse
	}

	return decisions, nil
}
