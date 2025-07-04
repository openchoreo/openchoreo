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
					Port: 8080,
				},
				OpenSearch: OpenSearchConfig{
					Address: "http://localhost:9200",
					Timeout: 30 * time.Second,
				},
				Logging: LoggingConfig{
					MaxLogLimit: 1000,
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
					Port: 8080,
				},
				OpenSearch: OpenSearchConfig{
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
			name: "invalid opensearch timeout",
			config: Config{
				Server: ServerConfig{
					Port: 8080,
				},
				OpenSearch: OpenSearchConfig{
					Address: "http://localhost:9200",
					Timeout: 0,
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
					Port: 8080,
				},
				OpenSearch: OpenSearchConfig{
					Address: "http://localhost:9200",
					Timeout: 30 * time.Second,
				},
				Logging: LoggingConfig{
					MaxLogLimit: 0,
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
