// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/internal/logging"
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
}

// EndpointConfig holds a single webhook endpoint and its (optional)
// retry policy. When `Retry` is nil, the dispatcher tries exactly once
// and gives up on failure — the typical Backstage consumer reconciles
// missed events via its periodic full-sync, so retry isn't needed for
// the default case. Set this for endpoints that have no equivalent
// reconciliation mechanism.
type EndpointConfig struct {
	URL   string       `yaml:"url"`
	Retry *RetryConfig `yaml:"retry,omitempty"`
}

// RetryConfig holds retry settings for a single webhook endpoint.
type RetryConfig struct {
	MaxAttempts int `yaml:"maxAttempts"`
	BackoffMs   int `yaml:"backoffMs"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// ToLoggingConfig converts the YAML-shaped LoggingConfig into the
// shared logging package's Config so the event-forwarder uses the same
// logger construction as every other OpenChoreo binary.
func (l LoggingConfig) ToLoggingConfig() logging.Config {
	return logging.Config{Level: l.Level, Format: l.Format}
}

// Load reads config from a YAML file at the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	cfg := &Config{
		Server:  ServerConfig{Port: 8080},
		Logging: LoggingConfig{Level: "info"},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	for i, ep := range cfg.Webhooks.Endpoints {
		trimmed := strings.TrimSpace(ep.URL)
		if trimmed == "" {
			return nil, fmt.Errorf("webhooks.endpoints[%d]: url is required", i)
		}
		u, err := url.Parse(trimmed)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("webhooks.endpoints[%d]: invalid url %q", i, ep.URL)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, fmt.Errorf("webhooks.endpoints[%d]: unsupported scheme %q (want http or https)", i, u.Scheme)
		}
		if ep.Retry != nil {
			if ep.Retry.MaxAttempts < 1 {
				return nil, fmt.Errorf("webhooks.endpoints[%d].retry.maxAttempts must be >= 1", i)
			}
			if ep.Retry.BackoffMs < 0 {
				return nil, fmt.Errorf("webhooks.endpoints[%d].retry.backoffMs must be >= 0", i)
			}
		}
	}

	return cfg, nil
}
