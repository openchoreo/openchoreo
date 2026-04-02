// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
)

// APIClient provides HTTP client for API servers (used by ObserverClient)
type APIClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func (c *APIClient) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	// Check if token needs refresh before making request
	if c.token != "" && auth.IsTokenExpired(c.token) {
		newToken, err := auth.RefreshToken()
		if err != nil {
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
		c.token = newToken
	}

	url := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}
