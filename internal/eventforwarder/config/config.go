// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/openchoreo/openchoreo/internal/logging"
)

// Config holds the event-forwarder configuration.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Webhooks WebhooksConfig `yaml:"webhooks"`
	Watch    WatchConfig    `yaml:"watch"`
	Logging  LoggingConfig  `yaml:"logging"`
}

// WatchConfig declares which Kubernetes resources the forwarder watches.
// The list is authoritative — if `resources` is empty, the forwarder
// watches nothing besides the always-on, label-filtered Namespace
// informer (Namespaces are handled separately because they carry the
// `openchoreo.dev/control-plane=true` label selector).
type WatchConfig struct {
	Resources []ResourceConfig `yaml:"resources"`
}

// ResourceConfig identifies a single Kubernetes resource to watch by
// its API GroupVersionResource — the same form `kubectl api-resources`
// prints. The plural-lowercase `resource` field is required because
// the informer factory consumes GVRs directly; no Kind-to-Resource
// discovery is performed at startup.
//
// `labelSelector` is optional. When set, the K8s API server applies
// the selector server-side during list/watch, so the informer cache
// only ever holds matching objects. The forwarder uses this to scope
// the core Namespace informer to OpenChoreo Organizations (label
// `openchoreo.dev/control-plane=true`) so events for kube-system,
// cert-manager, data-plane, and other ambient namespaces never reach
// the dispatcher. Operators can apply the same mechanism to any
// other watched resource — e.g. scoping `components` to a team label.
type ResourceConfig struct {
	Group         string `yaml:"group"`
	Version       string `yaml:"version"`
	Resource      string `yaml:"resource"`
	LabelSelector string `yaml:"labelSelector,omitempty"`
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

	for i, r := range cfg.Watch.Resources {
		// `group` may legitimately be empty (the core "" group, e.g. v1
		// resources like ConfigMaps). `version` and `resource` are not.
		if strings.TrimSpace(r.Version) == "" {
			return nil, fmt.Errorf("watch.resources[%d]: version is required", i)
		}
		if strings.TrimSpace(r.Resource) == "" {
			return nil, fmt.Errorf("watch.resources[%d]: resource is required (lowercase plural, e.g. \"projects\")", i)
		}
		// Parse the label selector up front so a typo surfaces at
		// startup with a clear error, not as a cryptic informer
		// failure mid-flight after pods are already running.
		if strings.TrimSpace(r.LabelSelector) != "" {
			if _, err := labels.Parse(r.LabelSelector); err != nil {
				return nil, fmt.Errorf("watch.resources[%d]: invalid labelSelector %q: %w", i, r.LabelSelector, err)
			}
		}
	}

	return cfg, nil
}
