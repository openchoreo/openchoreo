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
)

// Config holds all configuration for the logging service
type Config struct {
	Server     ServerConfig     `koanf:"server"`
	OpenSearch OpenSearchConfig `koanf:"opensearch"`
	Auth       AuthConfig       `koanf:"auth"`
	Logging    LoggingConfig    `koanf:"logging"`
	LogLevel   string           `koanf:"loglevel"`
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

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret    string `koanf:"jwt.secret"`
	EnableAuth   bool   `koanf:"enable.auth"`
	RequiredRole string `koanf:"required.role"`
}

// LoggingConfig holds application logging configuration
type LoggingConfig struct {
	MaxLogLimit          int `koanf:"max.log.limit"`
	DefaultLogLimit      int `koanf:"default.log.limit"`
	DefaultBuildLogLimit int `koanf:"default.build.log.limit"`
	MaxLogLinesPerFile   int `koanf:"max.log.lines.per.file"`
}

// Load loads configuration from environment variables and defaults
func Load() (*Config, error) {
	k := koanf.New(".")

	// Load defaults first
	if err := k.Load(confmap.Provider(getDefaults(), "."), nil); err != nil {
		return nil, fmt.Errorf("failed to load defaults: %w", err)
	}

	// Load environment variables for specific keys we care about
	envOverrides := make(map[string]interface{})

	// Define environment variable mappings
	envMappings := map[string]string{
		"SERVER_PORT":                     "server.port",
		"SERVER_READ_TIMEOUT":             "server.read.timeout",
		"SERVER_WRITE_TIMEOUT":            "server.write.timeout",
		"SERVER_SHUTDOWN_TIMEOUT":         "server.shutdown.timeout",
		"OPENSEARCH_ADDRESS":              "opensearch.address",
		"OPENSEARCH_USERNAME":             "opensearch.username",
		"OPENSEARCH_PASSWORD":             "opensearch.password",
		"OPENSEARCH_TIMEOUT":              "opensearch.timeout",
		"OPENSEARCH_MAX_RETRIES":          "opensearch.max.retries",
		"OPENSEARCH_INDEX_PREFIX":         "opensearch.index.prefix",
		"OPENSEARCH_INDEX_PATTERN":        "opensearch.index.pattern",
		"OPENSEARCH_LEGACY_PATTERN":       "opensearch.legacy.pattern",
		"AUTH_JWT_SECRET":                 "auth.jwt.secret",
		"AUTH_ENABLE_AUTH":                "auth.enable.auth",
		"AUTH_REQUIRED_ROLE":              "auth.required.role",
		"LOGGING_MAX_LOG_LIMIT":           "logging.max.log.limit",
		"LOGGING_DEFAULT_LOG_LIMIT":       "logging.default.log.limit",
		"LOGGING_DEFAULT_BUILD_LOG_LIMIT": "logging.default.build.log.limit",
		"LOGGING_MAX_LOG_LINES_PER_FILE":  "logging.max.log.lines.per.file",
		"LOG_LEVEL":                       "loglevel",
		"PORT":                            "server.port",           // Common alias
		"JWT_SECRET":                      "auth.jwt.secret",       // Common alias
		"ENABLE_AUTH":                     "auth.enable.auth",      // Common alias
		"MAX_LOG_LIMIT":                   "logging.max.log.limit", // Common alias
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
			"index.prefix":   "kubernetes-",
			"index.pattern":  "kubernetes-*",
			"legacy.pattern": "choreo*",
		},
		"auth": map[string]interface{}{
			"enable.auth":   false,
			"jwt.secret":    "default-secret",
			"required.role": "user",
		},
		"logging": map[string]interface{}{
			"max.log.limit":           10000,
			"default.log.limit":       100,
			"default.build.log.limit": 3000,
			"max.log.lines.per.file":  600000,
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

	if c.Logging.MaxLogLimit <= 0 {
		return fmt.Errorf("max log limit must be positive")
	}

	return nil
}
