// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- PrepareWebhookNotificationConfig ---

func TestPrepareWebhookNotificationConfig(t *testing.T) {
	logger := discardLogger()

	tests := []struct {
		name          string
		configMapData map[string]string
		secretData    map[string][]byte
		wantErr       bool
		wantErrMsg    string
		check         func(t *testing.T, cfg WebhookConfig)
	}{
		{
			name:          "missing webhook URL returns error",
			configMapData: map[string]string{},
			wantErr:       true,
			wantErrMsg:    "webhook URL not found in ConfigMap",
		},
		{
			name:          "basic config with URL and no headers",
			configMapData: map[string]string{"webhook.url": "https://example.com/hook"},
			check: func(t *testing.T, cfg WebhookConfig) {
				assert.Equal(t, "https://example.com/hook", cfg.URL)
				assert.Empty(t, cfg.Headers)
				assert.Empty(t, cfg.PayloadTemplate)
			},
		},
		{
			name: "headers from ConfigMap inline values",
			configMapData: map[string]string{
				"webhook.url":            "https://example.com/hook",
				"webhook.headers":        "X-Token,X-Env",
				"webhook.header.X-Token": "tok123",
				"webhook.header.X-Env":   "prod",
			},
			check: func(t *testing.T, cfg WebhookConfig) {
				assert.Equal(t, "tok123", cfg.Headers["X-Token"])
				assert.Equal(t, "prod", cfg.Headers["X-Env"])
			},
		},
		{
			name: "headers resolved from Secret",
			configMapData: map[string]string{
				"webhook.url":     "https://example.com/hook",
				"webhook.headers": "Authorization",
			},
			secretData: map[string][]byte{
				"webhook.header.Authorization": []byte("Bearer secrettoken"),
			},
			check: func(t *testing.T, cfg WebhookConfig) {
				assert.Equal(t, "Bearer secrettoken", cfg.Headers["Authorization"])
			},
		},
		{
			name: "header key missing from both ConfigMap and Secret is omitted",
			configMapData: map[string]string{
				"webhook.url":     "https://example.com/hook",
				"webhook.headers": "X-Missing",
			},
			check: func(t *testing.T, cfg WebhookConfig) {
				_, exists := cfg.Headers["X-Missing"]
				assert.False(t, exists)
			},
		},
		{
			name: "empty key in comma-separated header list is skipped",
			configMapData: map[string]string{
				"webhook.url":            "https://example.com/hook",
				"webhook.headers":        "X-Token,,X-Env",
				"webhook.header.X-Token": "t",
				"webhook.header.X-Env":   "e",
			},
			check: func(t *testing.T, cfg WebhookConfig) {
				assert.Len(t, cfg.Headers, 2)
			},
		},
		{
			name: "payload template populated from ConfigMap",
			configMapData: map[string]string{
				"webhook.url":             "https://example.com/hook",
				"webhook.payloadTemplate": `{"alert": "alertName"}`,
			},
			check: func(t *testing.T, cfg WebhookConfig) {
				assert.Equal(t, `{"alert": "alertName"}`, cfg.PayloadTemplate)
			},
		},
		{
			name: "nil secret does not panic when headers reference secret",
			configMapData: map[string]string{
				"webhook.url":     "https://example.com/hook",
				"webhook.headers": "X-Token",
			},
			secretData: nil,
			check: func(t *testing.T, cfg WebhookConfig) {
				// Should complete without panic; header omitted since not in ConfigMap or Secret
				_, exists := cfg.Headers["X-Token"]
				assert.False(t, exists)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{Data: tc.configMapData}
			var secret *corev1.Secret
			if tc.secretData != nil {
				secret = &corev1.Secret{Data: tc.secretData}
			}

			cfg, err := PrepareWebhookNotificationConfig(cm, secret, logger)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrMsg)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, cfg)
			}
		})
	}
}

// --- SendWebhookWithConfig ---

func TestSendWebhookWithConfig_EmptyURL(t *testing.T) {
	err := SendWebhookWithConfig(context.Background(), &WebhookConfig{}, map[string]interface{}{"key": "val"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "webhook URL is required")
}

func TestSendWebhookWithConfig_Success(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		headers      map[string]string
		checkRequest func(t *testing.T, r *http.Request)
	}{
		{
			name:       "200 response returns nil",
			statusCode: http.StatusOK,
		},
		{
			name:       "201 response returns nil",
			statusCode: http.StatusCreated,
		},
		{
			name:       "custom headers forwarded",
			statusCode: http.StatusOK,
			headers:    map[string]string{"X-Custom": "value123"},
			checkRequest: func(t *testing.T, r *http.Request) {
				assert.Equal(t, "value123", r.Header.Get("X-Custom"))
			},
		},
		{
			name:       "Content-Type is always application/json",
			statusCode: http.StatusOK,
			checkRequest: func(t *testing.T, r *http.Request) {
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.checkRequest != nil {
					tc.checkRequest(t, r)
				}
				w.WriteHeader(tc.statusCode)
			}))
			defer ts.Close()

			origClient := httpClient
			t.Cleanup(func() { httpClient = origClient })
			httpClient = ts.Client()

			cfg := &WebhookConfig{URL: ts.URL, Headers: tc.headers}
			err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{"alert": "test"})
			assert.NoError(t, err)
		})
	}
}

func TestSendWebhookWithConfig_Non2xxResponse(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		wantErrContain string
	}{
		{
			name:           "404 response returns error with status code",
			statusCode:     http.StatusNotFound,
			wantErrContain: "404",
		},
		{
			name:           "500 response with body includes body in error",
			statusCode:     http.StatusInternalServerError,
			responseBody:   "internal server error details",
			wantErrContain: "internal server error details",
		},
		{
			name:           "500 response without body has status code in error",
			statusCode:     http.StatusInternalServerError,
			wantErrContain: "500",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				if tc.responseBody != "" {
					_, _ = io.WriteString(w, tc.responseBody)
				}
			}))
			defer ts.Close()

			origClient := httpClient
			t.Cleanup(func() { httpClient = origClient })
			httpClient = ts.Client()

			cfg := &WebhookConfig{URL: ts.URL}
			err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrContain)
		})
	}
}

func TestSendWebhookWithConfig_ConnectionRefused(t *testing.T) {
	// Use an address that will refuse connections
	cfg := &WebhookConfig{URL: "http://127.0.0.1:1/webhook"}
	err := SendWebhookWithConfig(context.Background(), cfg, map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send webhook request")
}

func TestSendWebhookWithConfig_RequestPayload(t *testing.T) {
	var capturedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	origClient := httpClient
	t.Cleanup(func() { httpClient = origClient })
	httpClient = ts.Client()

	input := map[string]interface{}{
		"alertName": "HighCPU",
		"severity":  "critical",
		"threshold": 90.0,
	}
	cfg := &WebhookConfig{URL: ts.URL}
	err := SendWebhookWithConfig(context.Background(), cfg, input)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedBody, &got))
	assert.Equal(t, "HighCPU", got["alertName"])
	assert.Equal(t, "critical", got["severity"])
	assert.Equal(t, 90.0, got["threshold"])
}
