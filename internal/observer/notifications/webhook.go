// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookConfig holds webhook configuration for sending alerts
type WebhookConfig struct {
	URL     string
	Headers map[string]string
}

// SendWebhookWithConfig sends an alert webhook using the provided configuration.
// It sends the alertDetails JSON object as the request body.
func SendWebhookWithConfig(ctx context.Context, config *WebhookConfig, alertDetails map[string]interface{}) error {
	if config.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	// Marshal alertDetails to JSON
	jsonBody, err := json.Marshal(alertDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal alert details to JSON: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.URL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set Content-Type header
	req.Header.Set("Content-Type", "application/json")

	// Add custom headers if provided
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook request failed with status code: %d", resp.StatusCode)
	}

	return nil
}
