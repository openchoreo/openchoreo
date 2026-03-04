// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"testing"
	"time"
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
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test defaults
	if cfg.Server.Port != 9097 {
		t.Errorf("Expected default port 9097, got %d", cfg.Server.Port)
	}

	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("Expected read timeout 30s, got %v", cfg.Server.ReadTimeout)
	}

	if cfg.OpenSearch.Address != "http://localhost:9200" {
		t.Errorf("Expected default OpenSearch address, got %s", cfg.OpenSearch.Address)
	}

	if cfg.OpenSearch.Username != "admin" {
		t.Errorf("Expected default username 'admin', got %s", cfg.OpenSearch.Username)
	}

	if cfg.LogLevel != "info" {
		t.Errorf("Expected default log level 'info', got %s", cfg.LogLevel)
	}

	if cfg.Auth.EnableAuth != false {
		t.Errorf("Expected auth disabled by default, got %t", cfg.Auth.EnableAuth)
	}

	if cfg.Logging.MaxLogLimit != 10000 {
		t.Errorf("Expected max log limit 10000, got %d", cfg.Logging.MaxLogLimit)
	}
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
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test environment variable overrides
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080 from env, got %d", cfg.Server.Port)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("Expected log level 'debug' from env, got %s", cfg.LogLevel)
	}

	if cfg.OpenSearch.Address != "https://opensearch.example.com:9200" {
		t.Errorf("Expected OpenSearch address from env, got %s", cfg.OpenSearch.Address)
	}

	if cfg.OpenSearch.Username != "testuser" {
		t.Errorf("Expected username 'testuser' from env, got %s", cfg.OpenSearch.Username)
	}

	if cfg.OpenSearch.Password != "testpass" {
		t.Errorf("Expected password 'testpass' from env, got %s", cfg.OpenSearch.Password)
	}

	if cfg.Auth.EnableAuth != true {
		t.Errorf("Expected auth enabled from env, got %t", cfg.Auth.EnableAuth)
	}

	if cfg.Logging.MaxLogLimit != 5000 {
		t.Errorf("Expected max log limit 5000 from env, got %d", cfg.Logging.MaxLogLimit)
	}
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
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			if len(cfg.CORS.AllowedOrigins) != len(tt.expected) {
				t.Fatalf("Expected %d origins, got %d: %v", len(tt.expected), len(cfg.CORS.AllowedOrigins), cfg.CORS.AllowedOrigins)
			}
			for i, want := range tt.expected {
				if cfg.CORS.AllowedOrigins[i] != want {
					t.Errorf("Origin[%d]: expected %q, got %q", i, want, cfg.CORS.AllowedOrigins[i])
				}
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
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			if cfg.Adapters.LogsAdapterEnabled != tt.expectedLogs {
				t.Errorf("LogsAdapterEnabled: expected %v, got %v (env=%q)",
					tt.expectedLogs, cfg.Adapters.LogsAdapterEnabled, tt.logsAdapterEnabled)
			}
			if cfg.Adapters.TracingAdapterEnabled != tt.expectedTracing {
				t.Errorf("TracingAdapterEnabled: expected %v, got %v (env=%q)",
					tt.expectedTracing, cfg.Adapters.TracingAdapterEnabled, tt.tracingAdapterEnabled)
			}
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
			if err == nil {
				t.Errorf("Expected error for invalid boolean value %q, but got none", tt.value)
			}
		})
	}
}

func TestLoad_BooleanEnvParsing_Unset(t *testing.T) {
	// When env vars are not set, adapters should default to false
	os.Unsetenv("LOGS_ADAPTER_ENABLED")
	os.Unsetenv("TRACING_ADAPTER_ENABLED")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Adapters.LogsAdapterEnabled != false {
		t.Errorf("Expected LogsAdapterEnabled default false, got %v", cfg.Adapters.LogsAdapterEnabled)
	}
	if cfg.Adapters.TracingAdapterEnabled != false {
		t.Errorf("Expected TracingAdapterEnabled default false, got %v", cfg.Adapters.TracingAdapterEnabled)
	}
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
			name: "missing prometheus address",
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
			},
			expectErr: true,
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
			if tt.expectErr && err == nil {
				t.Error("Expected validation error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}
