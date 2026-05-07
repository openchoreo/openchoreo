// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the event-forwarder configuration.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Webhooks WebhooksConfig `yaml:"webhooks"`
	Logging  LoggingConfig  `yaml:"logging"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port int `yaml:"port"`
}

// WebhooksConfig holds webhook dispatch settings.
type WebhooksConfig struct {
	Endpoints []EndpointConfig `yaml:"endpoints"`
	Retry     RetryConfig      `yaml:"retry"`
}

// EndpointConfig holds a single webhook endpoint URL.
type EndpointConfig struct {
	URL string `yaml:"url"`
}

// RetryConfig holds retry settings for webhook dispatch.
type RetryConfig struct {
	MaxAttempts int `yaml:"maxAttempts"`
	BackoffMs   int `yaml:"backoffMs"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// Load reads config from a YAML file at the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	cfg := &Config{
		Server: ServerConfig{Port: 8080},
		Webhooks: WebhooksConfig{
			Retry: RetryConfig{
				MaxAttempts: 3,
				BackoffMs:   1000,
			},
		},
		Logging: LoggingConfig{Level: "info"},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	return cfg, nil
}
