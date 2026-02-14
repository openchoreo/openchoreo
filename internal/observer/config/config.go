// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"
	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/subject"
)

// Config holds all configuration for the logging service
type Config struct {
	Server       ServerConfig       `koanf:"server"`
	OpenSearch   OpenSearchConfig   `koanf:"opensearch"`
	Prometheus   PrometheusConfig   `koanf:"prometheus"`
	Auth         AuthConfig         `koanf:"auth"`
	Authz        AuthzConfig        `koanf:"authz"`
	Logging      LoggingConfig      `koanf:"logging"`
	Alerting     AlertingConfig     `koanf:"alerting"`
	Experimental ExperimentalConfig `koanf:"experimental"`
	LogLevel     string             `koanf:"loglevel"`
}

// ExperimentalConfig holds experimental feature flags
type ExperimentalConfig struct {
	UseLogsBackend     bool          `koanf:"use.logs.backend"`
	LogsBackendURL     string        `koanf:"logs.backend.url"`
	LogsBackendTimeout time.Duration `koanf:"logs.backend.timeout"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port            int           `koanf:"port"`
	ReadTimeout     time.Duration `koanf:"read.timeout"`
	WriteTimeout    time.Duration `koanf:"write.timeout"`
	ShutdownTimeout time.Duration `koanf:"shutdown.timeout"`
}

// OpenSearchConfig holds OpenSearch connection configuration
type OpenSearchConfig struct {
	Address       string        `koanf:"address"`
	Username      string        `koanf:"username"`
	Password      string        `koanf:"password"`
	Timeout       time.Duration `koanf:"timeout"`
	MaxRetries    int           `koanf:"max.retries"`
	IndexPrefix   string        `koanf:"index.prefix"`
	IndexPattern  string        `koanf:"index.pattern"`
	LegacyPattern string        `koanf:"legacy.pattern"`
}

// PrometheusConfig holds Prometheus connection configuration
type PrometheusConfig struct {
	Address string        `koanf:"address"`
	Timeout time.Duration `koanf:"timeout"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret    string                   `koanf:"jwt.secret"`
	EnableAuth   bool                     `koanf:"enable.auth"`
	RequiredRole string                   `koanf:"required.role"`
	UserTypes    []subject.UserTypeConfig `koanf:"user_types"`
}

// AuthzConfig holds authorization configuration
type AuthzConfig struct {
	ServiceURL string        `koanf:"service.url"`
	Timeout    time.Duration `koanf:"timeout"`
}

// LoggingConfig holds application logging configuration
type LoggingConfig struct {
	MaxLogLimit          int `koanf:"max.log.limit"`
	DefaultLogLimit      int `koanf:"default.log.limit"`
	DefaultBuildLogLimit int `koanf:"default.build.log.limit"`
	MaxLogLinesPerFile   int `koanf:"max.log.lines.per.file"`
}

// AlertingConfig holds configuration related to alerting features
type AlertingConfig struct {
	// RCAServiceURL is the base URL for the AI RCA (Root Cause Analysis) service.
	// Used for health checks and triggering RCA analysis.
	RCAServiceURL string `koanf:"rca.service.url"`
	// ObservabilityNamespace is the Kubernetes namespace where openchoreo-observability-plane is deployed.
	// Used for creating/listing PrometheusRule CRs for metric-based alerting.
	ObservabilityNamespace string `koanf:"observability.namespace"`
}

// Load loads configuration from environment variables and defaults
func Load() (*Config, error) {
	k := koanf.New(".")

	// Load defaults first
	if err := k.Load(confmap.Provider(getDefaults(), "."), nil); err != nil {
		return nil, fmt.Errorf("failed to load defaults: %w", err)
	}

	// Load auth config file for JWT subject resolution
	authConfigPath := os.Getenv("OBSERVER_AUTH_CONFIG_PATH")
	if authConfigPath == "" {
		authConfigPath = "auth-config.yaml"
	}

	var authCfg struct {
		Auth struct {
			UserTypes []subject.UserTypeConfig `yaml:"user_types"`
		} `yaml:"auth"`
	}
	if _, err := os.Stat(authConfigPath); err == nil {
		data, err := os.ReadFile(authConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read auth config file: %w", err)
		}
		if err := yaml.Unmarshal(data, &authCfg); err != nil {
			return nil, fmt.Errorf("failed to parse auth config file: %w", err)
		}
	}

	// Load environment variables for specific keys we care about
	envOverrides := make(map[string]interface{})

	// Define environment variable mappings
	envMappings := map[string]string{
		"SERVER_PORT":                       "server.port",
		"SERVER_READ_TIMEOUT":               "server.read.timeout",
		"SERVER_WRITE_TIMEOUT":              "server.write.timeout",
		"SERVER_SHUTDOWN_TIMEOUT":           "server.shutdown.timeout",
		"OPENSEARCH_ADDRESS":                "opensearch.address",
		"OPENSEARCH_USERNAME":               "opensearch.username",
		"OPENSEARCH_PASSWORD":               "opensearch.password",
		"OPENSEARCH_TIMEOUT":                "opensearch.timeout",
		"OPENSEARCH_MAX_RETRIES":            "opensearch.max.retries",
		"OPENSEARCH_INDEX_PREFIX":           "opensearch.index.prefix",
		"OPENSEARCH_INDEX_PATTERN":          "opensearch.index.pattern",
		"OPENSEARCH_LEGACY_PATTERN":         "opensearch.legacy.pattern",
		"PROMETHEUS_ADDRESS":                "prometheus.address",
		"PROMETHEUS_TIMEOUT":                "prometheus.timeout",
		"AUTH_JWT_SECRET":                   "auth.jwt.secret",
		"AUTH_ENABLE_AUTH":                  "auth.enable.auth",
		"AUTH_REQUIRED_ROLE":                "auth.required.role",
		"AUTHZ_SERVICE_URL":                 "authz.service.url",
		"AUTHZ_TIMEOUT":                     "authz.timeout",
		"LOGGING_MAX_LOG_LIMIT":             "logging.max.log.limit",
		"LOGGING_DEFAULT_LOG_LIMIT":         "logging.default.log.limit",
		"LOGGING_DEFAULT_BUILD_LOG_LIMIT":   "logging.default.build.log.limit",
		"LOGGING_MAX_LOG_LINES_PER_FILE":    "logging.max.log.lines.per.file",
		"RCA_SERVICE_URL":                   "alerting.rca.service.url",
		"OBSERVABILITY_NAMESPACE":           "alerting.observability.namespace",
		"LOG_LEVEL":                         "loglevel",
		"PORT":                              "server.port",           // Common alias
		"JWT_SECRET":                        "auth.jwt.secret",       // Common alias
		"ENABLE_AUTH":                       "auth.enable.auth",      // Common alias
		"MAX_LOG_LIMIT":                     "logging.max.log.limit", // Common alias
		"EXPERIMENTAL_USE_LOGS_BACKEND":     "experimental.use.logs.backend",
		"EXPERIMENTAL_LOGS_BACKEND_URL":     "experimental.logs.backend.url",
		"EXPERIMENTAL_LOGS_BACKEND_TIMEOUT": "experimental.logs.backend.timeout",
	}

	// Check for environment variables and map them to nested structure
	for envKey, configKey := range envMappings {
		if value := os.Getenv(envKey); value != "" {
			// Split the config key and create nested structure
			parts := strings.Split(configKey, ".")
			if len(parts) == 1 {
				// Top-level key
				envOverrides[configKey] = value
			} else if len(parts) == 2 {
				// Nested key like "server.port"
				section := parts[0]
				key := parts[1]
				if envOverrides[section] == nil {
					envOverrides[section] = make(map[string]interface{})
				}
				envOverrides[section].(map[string]interface{})[key] = value
			} else if len(parts) >= 3 {
				// Handle multi-part keys like "logging.max.log.limit"
				section := parts[0]
				key := strings.Join(parts[1:], ".")
				if envOverrides[section] == nil {
					envOverrides[section] = make(map[string]interface{})
				}
				envOverrides[section].(map[string]interface{})[key] = value
			}
		}
	}

	// Load environment overrides
	if len(envOverrides) > 0 {
		if err := k.Load(confmap.Provider(envOverrides, "."), nil); err != nil {
			return nil, fmt.Errorf("failed to load environment overrides: %w", err)
		}
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Assign user types from separately loaded auth config
	cfg.Auth.UserTypes = authCfg.Auth.UserTypes

	// Validate and sort user types configuration
	if len(cfg.Auth.UserTypes) > 0 {
		if err := subject.ValidateConfig(cfg.Auth.UserTypes); err != nil {
			return nil, fmt.Errorf("invalid user type config: %w", err)
		}
		subject.SortByPriority(cfg.Auth.UserTypes)
	}

	// Validate configuration
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// getDefaults returns the default configuration values
func getDefaults() map[string]interface{} {
	return map[string]interface{}{
		"server": map[string]interface{}{
			"port":             9097,
			"read.timeout":     "30s",
			"write.timeout":    "30s",
			"shutdown.timeout": "10s",
		},
		"opensearch": map[string]interface{}{
			"address":        "http://localhost:9200",
			"username":       "admin",
			"password":       "admin",
			"timeout":        "180s",
			"max.retries":    3,
			"index.prefix":   "container-logs-",
			"index.pattern":  "container-logs-*",
			"legacy.pattern": "choreo*",
		},
		"prometheus": map[string]interface{}{
			"address": "http://localhost:9090",
			"timeout": "30s",
		},
		"auth": map[string]interface{}{
			"enable.auth":   false,
			"jwt.secret":    "default-secret",
			"required.role": "user",
		},
		"authz": map[string]interface{}{
			"service.url": "http://localhost:8080",
			"timeout":     "30s",
		},
		"logging": map[string]interface{}{
			"max.log.limit":           10000,
			"default.log.limit":       100,
			"default.build.log.limit": 3000,
			"max.log.lines.per.file":  600000,
		},
		"alerting": map[string]interface{}{
			"rca.service.url":         "http://ai-rca-agent:8080",
			"observability.namespace": "openchoreo-observability-plane",
		},
		"experimental": map[string]interface{}{
			"use.logs.backend":     false,
			"logs.backend.url":     "",
			"logs.backend.timeout": "30s",
		},
		"loglevel": "info",
	}
}

func (c *Config) validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.OpenSearch.Address == "" {
		return fmt.Errorf("opensearch address is required")
	}

	if c.OpenSearch.Timeout <= 0 {
		return fmt.Errorf("opensearch timeout must be positive")
	}

	if c.Prometheus.Address == "" {
		return fmt.Errorf("prometheus address is required")
	}

	if c.Prometheus.Timeout <= 0 {
		return fmt.Errorf("prometheus timeout must be positive")
	}

	if c.Logging.MaxLogLimit <= 0 {
		return fmt.Errorf("max log limit must be positive")
	}

	if c.Authz.ServiceURL == "" {
		return fmt.Errorf("authz service URL is required")
	}
	if c.Authz.Timeout <= 0 {
		return fmt.Errorf("authz timeout must be positive")
	}

	return nil
}
