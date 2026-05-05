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
	// Clear any existing environment variables
	envVars := []string{
		"SERVER_PORT", "LOG_LEVEL", "OPENSEARCH_ADDRESS",
		"OPENSEARCH_USERNAME", "OPENSEARCH_PASSWORD",
	}
	for _, env := range envVars {
		os.Unsetenv(env)
	}

	cfg, err := Load()
	require.NoError(t, err, "Failed to load config")

	// Test defaults
	assert.Equal(t, 9097, cfg.Server.Port)
	assert.Equal(t, 30*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, "http://localhost:9200", cfg.OpenSearch.Address)
	assert.Equal(t, "admin", cfg.OpenSearch.Username)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.False(t, cfg.Auth.EnableAuth)
	assert.Equal(t, 10000, cfg.Logging.MaxLogLimit)
}

func TestLoad_WithEnvironmentVariables(t *testing.T) {
	// Set environment variables
	os.Setenv("SERVER_PORT", "8080")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("OPENSEARCH_ADDRESS", "https://opensearch.example.com:9200")
	os.Setenv("OPENSEARCH_USERNAME", "testuser")
	os.Setenv("OPENSEARCH_PASSWORD", "testpass")
	os.Setenv("AUTH_ENABLE_AUTH", "true")
	os.Setenv("LOGGING_MAX_LOG_LIMIT", "5000")

	defer func() {
		// Clean up environment variables
		envVars := []string{
			"SERVER_PORT", "LOG_LEVEL", "OPENSEARCH_ADDRESS",
			"OPENSEARCH_USERNAME", "OPENSEARCH_PASSWORD",
			"AUTH_ENABLE_AUTH", "LOGGING_MAX_LOG_LIMIT",
		}
		for _, env := range envVars {
			os.Unsetenv(env)
		}
	}()

	cfg, err := Load()
	require.NoError(t, err, "Failed to load config")

	// Test environment variable overrides
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "https://opensearch.example.com:9200", cfg.OpenSearch.Address)
	assert.Equal(t, "testuser", cfg.OpenSearch.Username)
	assert.Equal(t, "testpass", cfg.OpenSearch.Password)
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

func TestLoad_BooleanEnvParsing(t *testing.T) {
	tests := []struct {
		name                  string
		logsAdapterEnabled    string
		tracingAdapterEnabled string
		expectedLogs          bool
		expectedTracing       bool
	}{
		{
			name:                  "true string",
			logsAdapterEnabled:    "true",
			tracingAdapterEnabled: "true",
			expectedLogs:          true,
			expectedTracing:       true,
		},
		{
			name:                  "false string",
			logsAdapterEnabled:    "false",
			tracingAdapterEnabled: "false",
			expectedLogs:          false,
			expectedTracing:       false,
		},
		{
			name:                  "1 is true",
			logsAdapterEnabled:    "1",
			tracingAdapterEnabled: "1",
			expectedLogs:          true,
			expectedTracing:       true,
		},
		{
			name:                  "0 is false",
			logsAdapterEnabled:    "0",
			tracingAdapterEnabled: "0",
			expectedLogs:          false,
			expectedTracing:       false,
		},
		{
			name:                  "mixed valid values",
			logsAdapterEnabled:    "true",
			tracingAdapterEnabled: "false",
			expectedLogs:          true,
			expectedTracing:       false,
		},
		{
			name:                  "TRUE uppercase",
			logsAdapterEnabled:    "TRUE",
			tracingAdapterEnabled: "FALSE",
			expectedLogs:          true,
			expectedTracing:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LOGS_ADAPTER_ENABLED", tt.logsAdapterEnabled)
			t.Setenv("TRACING_ADAPTER_ENABLED", tt.tracingAdapterEnabled)

			cfg, err := Load()
			require.NoError(t, err, "Failed to load config")

			assert.Equal(t, tt.expectedLogs, cfg.Adapters.LogsAdapterEnabled,
				"LogsAdapterEnabled (env=%q)", tt.logsAdapterEnabled)
			assert.Equal(t, tt.expectedTracing, cfg.Adapters.TracingAdapterEnabled,
				"TracingAdapterEnabled (env=%q)", tt.tracingAdapterEnabled)
		})
	}
}

func TestLoad_BooleanEnvParsing_InvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "yes is invalid", value: "yes"},
		{name: "no is invalid", value: "no"},
		{name: "arbitrary string is invalid", value: "notabool"},
		{name: "numeric non-boolean is invalid", value: "2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LOGS_ADAPTER_ENABLED", tt.value)
			t.Setenv("TRACING_ADAPTER_ENABLED", tt.value)

			_, err := Load()
			require.Error(t, err, "Expected error for invalid boolean value %q", tt.value)
		})
	}
}

func TestLoad_BooleanEnvParsing_Unset(t *testing.T) {
	// When env vars are not set, adapters should default to true
	os.Unsetenv("LOGS_ADAPTER_ENABLED")
	os.Unsetenv("TRACING_ADAPTER_ENABLED")

	cfg, err := Load()
	require.NoError(t, err, "Failed to load config")

	assert.True(t, cfg.Adapters.LogsAdapterEnabled)
	assert.True(t, cfg.Adapters.TracingAdapterEnabled)
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		expectErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Server: ServerConfig{
					Port:         8080,
					InternalPort: 8081,
				},
				OpenSearch: OpenSearchConfig{
					Address: "http://localhost:9200",
					Timeout: 30 * time.Second,
				},
				Prometheus: PrometheusConfig{
					Address: "http://localhost:9090",
					Timeout: 30 * time.Second,
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
			},
			expectErr: false,
		},
		{
			name: "invalid port - too low",
			config: Config{
				Server: ServerConfig{
					Port: 0,
				},
				OpenSearch: OpenSearchConfig{
					Address: "http://localhost:9200",
					Timeout: 30 * time.Second,
				},
				Prometheus: PrometheusConfig{
					Address: "http://localhost:9090",
					Timeout: 30 * time.Second,
				},
				Logging: LoggingConfig{
					MaxLogLimit: 1000,
				},
			},
			expectErr: true,
		},
		{
			name: "invalid port - too high",
			config: Config{
				Server: ServerConfig{
					Port: 99999,
				},
				OpenSearch: OpenSearchConfig{
					Address: "http://localhost:9200",
					Timeout: 30 * time.Second,
				},
				Prometheus: PrometheusConfig{
					Address: "http://localhost:9090",
					Timeout: 30 * time.Second,
				},
				Logging: LoggingConfig{
					MaxLogLimit: 1000,
				},
			},
			expectErr: true,
		},
		{
			name: "missing opensearch address",
			config: Config{
				Server: ServerConfig{
					Port:         8080,
					InternalPort: 8081,
				},
				OpenSearch: OpenSearchConfig{
					Address: "",
					Timeout: 30 * time.Second,
				},
				Prometheus: PrometheusConfig{
					Address: "http://localhost:9090",
					Timeout: 30 * time.Second,
				},
				Logging: LoggingConfig{
					MaxLogLimit: 1000,
				},
			},
			expectErr: true,
		},
		{
			name: "invalid opensearch timeout",
			config: Config{
				Server: ServerConfig{
					Port:         8080,
					InternalPort: 8081,
				},
				OpenSearch: OpenSearchConfig{
					Address: "http://localhost:9200",
					Timeout: 0,
				},
				Prometheus: PrometheusConfig{
					Address: "http://localhost:9090",
					Timeout: 30 * time.Second,
				},
				Logging: LoggingConfig{
					MaxLogLimit: 1000,
				},
			},
			expectErr: true,
		},
		{
			name: "invalid max log limit",
			config: Config{
				Server: ServerConfig{
					Port:         8080,
					InternalPort: 8081,
				},
				OpenSearch: OpenSearchConfig{
					Address: "http://localhost:9200",
					Timeout: 30 * time.Second,
				},
				Prometheus: PrometheusConfig{
					Address: "http://localhost:9090",
					Timeout: 30 * time.Second,
				},
				Logging: LoggingConfig{
					MaxLogLimit: 0,
				},
			},
			expectErr: true,
		},
		{
			name: "prometheus address optional - empty is valid",
			config: Config{
				Server: ServerConfig{
					Port:         8080,
					InternalPort: 8081,
				},
				OpenSearch: OpenSearchConfig{
					Address: "http://localhost:9200",
					Timeout: 30 * time.Second,
				},
				Prometheus: PrometheusConfig{
					Address: "",
					Timeout: 30 * time.Second,
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
			},
			expectErr: false,
		},
		{
			name: "invalid prometheus timeout",
			config: Config{
				Server: ServerConfig{
					Port:         8080,
					InternalPort: 8081,
				},
				OpenSearch: OpenSearchConfig{
					Address: "http://localhost:9200",
					Timeout: 30 * time.Second,
				},
				Prometheus: PrometheusConfig{
					Address: "http://localhost:9090",
					Timeout: 0,
				},
				Logging: LoggingConfig{
					MaxLogLimit: 1000,
				},
			},
			expectErr: true,
		},
		{
			name: "missing authz service URL",
			config: Config{
				Server: ServerConfig{
					Port:         8080,
					InternalPort: 8081,
				},
				OpenSearch: OpenSearchConfig{
					Address: "http://localhost:9200",
					Timeout: 30 * time.Second,
				},
				Prometheus: PrometheusConfig{
					Address: "http://localhost:9090",
					Timeout: 30 * time.Second,
				},
				Logging: LoggingConfig{
					MaxLogLimit: 1000,
				},
				Authz: AuthzConfig{
					ServiceURL: "",
					Timeout:    30 * time.Second,
				},
			},
			expectErr: true,
		},
		{
			name: "invalid authz timeout",
			config: Config{
				Server: ServerConfig{
					Port:         8080,
					InternalPort: 8081,
				},
				OpenSearch: OpenSearchConfig{
					Address: "http://localhost:9200",
					Timeout: 30 * time.Second,
				},
				Prometheus: PrometheusConfig{
					Address: "http://localhost:9090",
					Timeout: 30 * time.Second,
				},
				Logging: LoggingConfig{
					MaxLogLimit: 1000,
				},
				Authz: AuthzConfig{
					ServiceURL: "http://localhost:8081",
					Timeout:    0,
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
