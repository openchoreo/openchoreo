// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Shared HTTP client for webhook notifications.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

const (
	// webhookURLKey holds the webhook URL: in the ConfigMap when it is inline, in the Secret when it
	// comes from a secret reference.
	webhookURLKey = "webhook.url"

	// redactedPlaceholder stands in for the parts of a webhook URL that can carry a credential.
	redactedPlaceholder = "<redacted>"

	// minScrubbedPieceLen is the shortest piece of a webhook URL's path or query that is removed
	// from text on its own. Shorter pieces are too common in ordinary text to be worth removing.
	minScrubbedPieceLen = 4
)

// WebhookConfig holds webhook configuration for sending alerts
type WebhookConfig struct {
	// URL can carry a credential in its path or query. Log it only through RedactWebhookURL.
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
	if err := validateWebhookURL(config.URL); err != nil {
		return err
	}

	// Marshal alertDetails to JSON
	jsonBody, err := json.Marshal(alertDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal alert details to JSON: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.URL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request for %s: %w", RedactWebhookURL(config.URL), scrubbedCause(err, config.URL))
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
		return fmt.Errorf("failed to send webhook request to %s: %w", RedactWebhookURL(config.URL), scrubbedCause(err, config.URL))
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read response body for better error messages
		var responseBody bytes.Buffer
		if _, readErr := responseBody.ReadFrom(resp.Body); readErr == nil && responseBody.Len() > 0 {
			return fmt.Errorf("webhook request failed with status code: %d, response: %s",
				resp.StatusCode, scrubWebhookURL(responseBody.String(), config.URL))
		}
		return fmt.Errorf("webhook request failed with status code: %d", resp.StatusCode)
	}

	return nil
}

// RedactWebhookURL returns the URL with everything but its scheme and host replaced, so that it can
// be logged. A value that is not an http(s) URL is replaced entirely, since it may be a secret
// that is not a URL at all.
func RedactWebhookURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || !isHTTPURL(u) {
		return redactedPlaceholder
	}
	return u.Scheme + "://" + u.Host + "/" + redactedPlaceholder
}

// validateWebhookURL checks that the URL is one a webhook can be sent to. Its errors leave the
// value out, including the parse error, which quotes parts of it.
func validateWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("webhook URL is not a valid URL")
	}
	if !isHTTPURL(u) {
		return errors.New("webhook URL must be an http or https URL with a host")
	}
	return nil
}

func isHTTPURL(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// scrubbedCause returns the cause of an error from net/http or net/url without the webhook URL.
// Those errors carry the URL, and their cause can quote parts of it too, as the error for a
// redirect with a malformed Location does.
func scrubbedCause(err error, rawURL string) error {
	for {
		var urlErr *url.Error
		if !errors.As(err, &urlErr) {
			break
		}
		err = urlErr.Err
	}
	if scrubbed := scrubWebhookURL(err.Error(), rawURL); scrubbed != err.Error() {
		return errors.New(scrubbed)
	}
	return err
}

// scrubWebhookURL removes the webhook URL from text that may quote it, such as a receiver's
// error page. Besides the URL, its path and its query, the pieces of the path and query are
// removed one by one, so that they are caught however the text joins or escapes them.
func scrubWebhookURL(text, rawURL string) string {
	for _, part := range webhookURLParts(rawURL) {
		text = strings.ReplaceAll(text, part, redactedPlaceholder)
	}
	return text
}

// webhookURLParts returns what scrubWebhookURL removes for the URL, longest first.
func webhookURLParts(rawURL string) []string {
	seen := map[string]struct{}{}
	var parts []string
	add := func(part string, minLen int) {
		if _, ok := seen[part]; ok || len(part) < minLen {
			return
		}
		seen[part] = struct{}{}
		parts = append(parts, part)
	}

	// "/" and "" are not worth hiding, and replacing them would mangle the text.
	add(rawURL, 2)
	if u, err := url.Parse(rawURL); err == nil {
		for _, whole := range []string{u.RequestURI(), u.EscapedPath(), u.Path, u.RawQuery} {
			add(whole, 2)
		}

		pieces := strings.Split(u.EscapedPath(), "/")
		pieces = append(pieces, strings.FieldsFunc(u.RawQuery, func(r rune) bool { return r == '&' || r == ';' || r == '=' })...)
		if password, ok := u.User.Password(); ok {
			pieces = append(pieces, u.User.Username(), password)
		}
		for _, piece := range pieces {
			add(piece, minScrubbedPieceLen)
			if unescaped, err := url.PathUnescape(piece); err == nil {
				add(unescaped, minScrubbedPieceLen)
			}
			if unescaped, err := url.QueryUnescape(piece); err == nil {
				add(unescaped, minScrubbedPieceLen)
			}
		}
	}

	sort.SliceStable(parts, func(i, j int) bool { return len(parts[i]) > len(parts[j]) })
	return parts
}

// PrepareWebhookNotificationConfig prepares webhook notification configuration from ConfigMap and Secret
func PrepareWebhookNotificationConfig(configMap *corev1.ConfigMap, secret *corev1.Secret, logger *slog.Logger) (WebhookConfig, error) {
	// The URL is in the ConfigMap when it is inline, and in the Secret when it comes from a secret
	// reference. Secret values often end in a newline.
	webhookURL := configMap.Data[webhookURLKey]
	if webhookURL == "" && secret != nil {
		webhookURL = strings.TrimSpace(string(secret.Data[webhookURLKey]))
	}
	if webhookURL == "" {
		return WebhookConfig{}, fmt.Errorf("webhook URL not found in ConfigMap or Secret")
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
		"url", RedactWebhookURL(webhookConfig.URL),
		"headerCount", len(webhookConfig.Headers),
		"hasPayloadTemplate", payloadTemplate != "")

	return webhookConfig, nil
}
