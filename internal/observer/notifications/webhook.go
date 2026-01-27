// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Shared HTTP client for webhook notifications.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// WebhookConfig holds webhook configuration for sending alerts
type WebhookConfig struct {
	URL             string
	Headers         map[string]string
	PayloadTemplate string // Optional JSON template with CEL expressions
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

	// Send the request using the shared HTTP client
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read response body for better error messages
		var responseBody bytes.Buffer
		if _, readErr := responseBody.ReadFrom(resp.Body); readErr == nil && responseBody.Len() > 0 {
			return fmt.Errorf("webhook request failed with status code: %d, response: %s", resp.StatusCode, responseBody.String())
		}
		return fmt.Errorf("webhook request failed with status code: %d", resp.StatusCode)
	}

	return nil
}

// PrepareWebhookNotificationConfig prepares webhook notification configuration from ConfigMap and Secret
func PrepareWebhookNotificationConfig(configMap *corev1.ConfigMap, secret *corev1.Secret, logger *slog.Logger) (WebhookConfig, error) {
	// Parse webhook URL
	webhookURL := configMap.Data["webhook.url"]
	if webhookURL == "" {
		return WebhookConfig{}, fmt.Errorf("webhook URL not found in ConfigMap")
	}

	// Parse headers from ConfigMap and Secret
	headers := make(map[string]string)
	if headerKeysStr, ok := configMap.Data["webhook.headers"]; ok && headerKeysStr != "" {
		headerKeys := strings.Split(headerKeysStr, ",")
		for _, key := range headerKeys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			// Try to get inline value from ConfigMap first
			if inlineValue, ok := configMap.Data[fmt.Sprintf("webhook.header.%s", key)]; ok {
				headers[key] = inlineValue
			} else if secret != nil && secret.Data != nil {
				// Try to get value from Secret (for secret-referenced headers)
				if secretValue, ok := secret.Data[fmt.Sprintf("webhook.header.%s", key)]; ok {
					headers[key] = string(secretValue)
				}
			}
		}
	}

	// Parse payload template if provided
	payloadTemplate := configMap.Data["webhook.payloadTemplate"]

	webhookConfig := WebhookConfig{
		URL:             webhookURL,
		Headers:         headers,
		PayloadTemplate: payloadTemplate,
	}

	logger.Debug("Final webhook config",
		"url", webhookConfig.URL,
		"headerCount", len(webhookConfig.Headers),
		"hasPayloadTemplate", payloadTemplate != "")

	return webhookConfig, nil
}
