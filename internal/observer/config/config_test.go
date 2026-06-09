// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_WithDefaults(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err, "Failed to load config")

	assert.Equal(t, 9097, cfg.Server.Port)
	assert.Equal(t, 30*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.False(t, cfg.Auth.EnableAuth)
	assert.Equal(t, 10000, cfg.Logging.MaxLogLimit)
	assert.Equal(t, "http://logs-adapter:9098", cfg.Adapters.LogsAdapterURL)
	assert.Equal(t, "http://tracing-adapter:9100", cfg.Adapters.TracingAdapterURL)
	assert.Equal(t, "http://metrics-adapter:9099", cfg.Adapters.MetricsAdapterURL)
}

func TestLoad_WithEnvironmentVariables(t *testing.T) {
	t.Setenv("SERVER_PORT", "8080")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("AUTH_ENABLE_AUTH", "true")
	t.Setenv("LOGGING_MAX_LOG_LIMIT", "5000")

	cfg, err := Load()
	require.NoError(t, err, "Failed to load config")

	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.True(t, cfg.Auth.EnableAuth)
	assert.Equal(t, 5000, cfg.Logging.MaxLogLimit)
}

func TestLoad_CORSAllowedOrigins(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected []string
	}{
		{
			name:     "simple comma-separated origins",
			envValue: "http://localhost:3000,http://example.com",
			expected: []string{"http://localhost:3000", "http://example.com"},
		},
		{
			name:     "whitespace is trimmed",
			envValue: " http://a.com , http://b.com ",
			expected: []string{"http://a.com", "http://b.com"},
		},
		{
			name:     "trailing comma produces no empty items",
			envValue: "http://a.com,http://b.com,",
			expected: []string{"http://a.com", "http://b.com"},
		},
		{
			name:     "multiple trailing commas",
			envValue: "http://a.com,,http://b.com,,",
			expected: []string{"http://a.com", "http://b.com"},
		},
		{
			name:     "whitespace-only entries are filtered",
			envValue: "http://a.com, , ,http://b.com",
			expected: []string{"http://a.com", "http://b.com"},
		},
		{
			name:     "single origin",
			envValue: "http://localhost:3000",
			expected: []string{"http://localhost:3000"},
		},
		{
			name:     "unset env var leaves empty slice",
			envValue: "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("CORS_ALLOWED_ORIGINS", tt.envValue)
			} else {
				os.Unsetenv("CORS_ALLOWED_ORIGINS")
			}

			cfg, err := Load()
			require.NoError(t, err, "Failed to load config")

			require.Len(t, cfg.CORS.AllowedOrigins, len(tt.expected))
			for i, want := range tt.expected {
				assert.Equal(t, want, cfg.CORS.AllowedOrigins[i], "Origin[%d]", i)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	baseValidConfig := func() Config {
		return Config{
			Server: ServerConfig{
				Port:         8080,
				InternalPort: 8081,
			},
			Logging: LoggingConfig{
				MaxLogLimit: 1000,
			},
			Authz: AuthzConfig{
				ServiceURL: "http://localhost:8081",
				Timeout:    30 * time.Second,
			},
			UIDResolver: UIDResolverConfig{
				OpenChoreoAPIURL:  "http://localhost:9099",
				OAuthTokenURL:     "http://localhost:8080/oauth2/token",
				OAuthClientID:     "test-client",
				OAuthClientSecret: "test-secret",
				Timeout:           30 * time.Second,
			},
			Adapters: AdaptersConfig{
				MetricsAdapterURL:     "http://localhost:9090",
				MetricsAdapterTimeout: 30 * time.Second,
			},
		}
	}

	tests := []struct {
		name      string
		mutate    func(c *Config)
		expectErr bool
	}{
		{
			name:      "valid config",
			mutate:    func(c *Config) {},
			expectErr: false,
		},
		{
			name:      "invalid port - too low",
			mutate:    func(c *Config) { c.Server.Port = 0 },
			expectErr: true,
		},
		{
			name:      "invalid port - too high",
			mutate:    func(c *Config) { c.Server.Port = 99999 },
			expectErr: true,
		},
		{
			name:      "invalid max log limit",
			mutate:    func(c *Config) { c.Logging.MaxLogLimit = 0 },
			expectErr: true,
		},
		{
			name:      "missing authz service URL",
			mutate:    func(c *Config) { c.Authz.ServiceURL = "" },
			expectErr: true,
		},
		{
			name:      "invalid authz timeout",
			mutate:    func(c *Config) { c.Authz.Timeout = 0 },
			expectErr: true,
		},
		{
			name:      "missing metrics adapter URL",
			mutate:    func(c *Config) { c.Adapters.MetricsAdapterURL = "" },
			expectErr: true,
		},
		{
			name:      "invalid metrics adapter timeout",
			mutate:    func(c *Config) { c.Adapters.MetricsAdapterTimeout = 0 },
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseValidConfig()
			tt.mutate(&cfg)
			err := cfg.validate()
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
