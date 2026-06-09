// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestLoad_FullConfig(t *testing.T) {
	path := writeTempConfig(t, `
server:
  port: 9090
webhooks:
  endpoints:
    - url: http://example.com/webhook-a
    - url: http://example.com/webhook-b
      retry:
        maxAttempts: 5
        backoffMs: 500
logging:
  level: debug
`)

	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, 9090, cfg.Server.Port)
	require.Len(t, cfg.Webhooks.Endpoints, 2)
	assert.Equal(t, "http://example.com/webhook-a", cfg.Webhooks.Endpoints[0].URL)
	assert.Nil(t, cfg.Webhooks.Endpoints[0].Retry,
		"endpoint without retry block must keep nil retry (try-once default)")
	assert.Equal(t, "http://example.com/webhook-b", cfg.Webhooks.Endpoints[1].URL)
	require.NotNil(t, cfg.Webhooks.Endpoints[1].Retry)
	assert.Equal(t, 5, cfg.Webhooks.Endpoints[1].Retry.MaxAttempts)
	assert.Equal(t, 500, cfg.Webhooks.Endpoints[1].Retry.BackoffMs)
	assert.Equal(t, "debug", cfg.Logging.Level)
}

func TestLoad_AppliesDefaultsWhenFieldsMissing(t *testing.T) {
	// An almost-empty config should still produce sensible defaults that
	// match what the production Load() seeds before YAML unmarshal.
	path := writeTempConfig(t, `
webhooks:
  endpoints:
    - url: http://example.com/webhook
`)

	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Server.Port, "default server port")
	assert.Nil(t, cfg.Webhooks.Endpoints[0].Retry,
		"endpoints default to no retry — consumers reconcile via their own full sync")
	assert.Equal(t, "info", cfg.Logging.Level, "default log level")
}

func TestLoad_EmptyFile(t *testing.T) {
	path := writeTempConfig(t, "")

	cfg, err := Load(path)
	require.NoError(t, err, "empty file should not error; defaults are applied")

	// Every default should be present.
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Empty(t, cfg.Webhooks.Endpoints, "no endpoints configured")
}

func TestLoad_RejectsInvalidRetryConfig(t *testing.T) {
	t.Run("zero maxAttempts", func(t *testing.T) {
		path := writeTempConfig(t, `
webhooks:
  endpoints:
    - url: http://example.com/webhook
      retry:
        maxAttempts: 0
        backoffMs: 100
`)
		_, err := Load(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maxAttempts must be >= 1")
	})

	t.Run("negative backoffMs", func(t *testing.T) {
		path := writeTempConfig(t, `
webhooks:
  endpoints:
    - url: http://example.com/webhook
      retry:
        maxAttempts: 3
        backoffMs: -100
`)
		_, err := Load(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "backoffMs must be >= 0")
	})
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/this/path/does/not/exist.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestLoad_RejectsInvalidWebhookURLs(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "empty url",
			body: `
webhooks:
  endpoints:
    - url: ""
`,
			wantErr: "url is required",
		},
		{
			name: "no scheme",
			body: `
webhooks:
  endpoints:
    - url: "example.com/webhook"
`,
			wantErr: "invalid url",
		},
		{
			name: "unsupported scheme",
			body: `
webhooks:
  endpoints:
    - url: "ftp://example.com/webhook"
`,
			wantErr: "unsupported scheme",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempConfig(t, tt.body)
			_, err := Load(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	// Tabs are not valid YAML indentation.
	path := writeTempConfig(t, "\tnot: valid\n")

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config file")
}
