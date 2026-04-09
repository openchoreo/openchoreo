// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/rca-agent/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
)

const evaluatesEndpoint = "/api/v1/authz/evaluates"

// Client is an HTTP client for the external authorization service.
type Client struct {
	httpClient *http.Client
	baseURL    string
	logger     *slog.Logger
}

// NewClient creates a new authz HTTP client.
func NewClient(cfg *config.AuthzConfig, logger *slog.Logger) (*Client, error) {
	if cfg.ServiceURL == "" {
		return nil, fmt.Errorf("authz service URL is required")
	}

	if cfg.Timeout <= 0 {
		return nil, fmt.Errorf("authz timeout must be positive")
	}

	httpClient := &http.Client{
		Timeout: time.Duration(cfg.Timeout) * time.Second,
	}
	if cfg.TLSInsecureSkipVerify {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // intentional for self-signed certs
		}
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

// Evaluate evaluates a single authorization request.
func (c *Client) Evaluate(ctx context.Context, request *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	if request == nil {
		return nil, fmt.Errorf("evaluate request must not be nil")
	}

	decisions, err := c.evaluate(ctx, []authzcore.EvaluateRequest{*request})
	if err != nil {
		return nil, err
	}

	if len(decisions) != 1 {
		c.logger.Error("Authz service returned unexpected decisions count", "count", len(decisions))
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

// BatchEvaluate evaluates multiple authorization requests.
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

	return &authzcore.BatchEvaluateResponse{Decisions: decisions}, nil
}

// GetSubjectProfile is not implemented for the RCA agent.
func (c *Client) GetSubjectProfile(_ context.Context, _ *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, errors.New("GetSubjectProfile is not supported in RCA agent")
}

func (c *Client) evaluate(ctx context.Context, requests []authzcore.EvaluateRequest) ([]authzcore.Decision, error) {
	body, err := json.Marshal(requests)
	if err != nil {
		c.logger.Error("Failed to marshal evaluate request", "error", err)
		return nil, fmt.Errorf("failed to marshal evaluate request: %w", err)
	}

	url := strings.TrimRight(c.baseURL, "/") + evaluatesEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		c.logger.Error("Failed to create HTTP request", "error", err)
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Forward the authentication token from the incoming request context
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
		if resp.StatusCode >= 500 {
			return nil, ErrAuthzServiceUnavailable
		}
		return nil, fmt.Errorf("authz service returned %d", resp.StatusCode)
	}

	var decisions []authzcore.Decision
	if err := json.Unmarshal(bodyBytes, &decisions); err != nil {
		c.logger.Error("Failed to decode authz response", "error", err, "body", string(bodyBytes))
		return nil, ErrAuthzInvalidResponse
	}

	return decisions, nil
}
