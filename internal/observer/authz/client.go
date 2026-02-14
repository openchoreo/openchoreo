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

type Client struct {
	httpClient *http.Client
	baseURL    string
	logger     *slog.Logger
}

// AuthzResponse represents the wrapped response from the authz service
type AuthzResponse struct {
	Success bool               `json:"success"`
	Data    authzcore.Decision `json:"data"`
}

// BatchAuthzResponse represents the wrapped response from the authz service for batch evaluate
type BatchAuthzResponse struct {
	Success bool                            `json:"success"`
	Data    authzcore.BatchEvaluateResponse `json:"data"`
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

// Evaluate evaluates a single authorization request
func (c *Client) Evaluate(ctx context.Context, request *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	body, err := json.Marshal(request)
	if err != nil {
		c.logger.Error("failed to marshal evaluate request", "error", err)
		return nil, fmt.Errorf("failed to marshal evaluate request: %w", err)
	}

	c.logger.Debug("Authz Request Body", "json", string(body))

	url := c.baseURL + "/api/v1/authz/evaluate"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		c.logger.Error("failed to create HTTP request", "error", err)
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

	if resp.StatusCode == http.StatusUnauthorized {
		c.logger.Warn("Authz service returned unauthorized", "status", resp.StatusCode)
		return nil, ErrAuthzUnauthorized
	}

	if resp.StatusCode == http.StatusForbidden {
		c.logger.Debug(ErrAuthzForbidden.Error())
		return &authzcore.Decision{Decision: false}, nil
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.logger.Error("Authz service returned error", "status", resp.StatusCode, "response_body", string(bodyBytes))
		return nil, fmt.Errorf("authz service returned %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("Failed to read authz response body", "error", err)
		return nil, ErrAuthzInvalidResponse
	}
	var response AuthzResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		c.logger.Error("Failed to decode authz response", "error", err, "body", string(bodyBytes))
		return nil, ErrAuthzInvalidResponse
	}

	c.logger.Debug("Authorization evaluated",
		"action", request.Action,
		"resource_type", request.Resource.Type,
		"resource_id", request.Resource.ID,
		"decision", response.Data.Decision,
		"reason", response.Data.Context)

	return &response.Data, nil
}

// BatchEvaluate evaluates multiple authorization requests
func (c *Client) BatchEvaluate(ctx context.Context, request *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
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
	req.Header.Set("Content-Type", "application/json")

	// Extract and forward the authentication token from the incoming request context
	if token := jwt.GetTokenFromContext(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

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
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.logger.Error("Authz service returned error", "status", resp.StatusCode, "response_body", string(bodyBytes))
		return nil, fmt.Errorf("authz service returned %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("Failed to read batch authz response body", "error", err)
		return nil, ErrAuthzInvalidResponse
	}

	var response BatchAuthzResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		c.logger.Error("Failed to decode batch authz response", "error", err, "body", string(bodyBytes))
		return nil, ErrAuthzInvalidResponse
	}

	c.logger.Debug("Batch authorization evaluated", "request_count", len(request.Requests))

	return &response.Data, nil
}

// GetSubjectProfile is not implemented for observer API
func (c *Client) GetSubjectProfile(ctx context.Context, request *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, errors.New("GetSubjectProfile is not supported in observer API")
}
