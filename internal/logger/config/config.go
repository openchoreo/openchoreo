// Copyright (c) 2025 openchoreo
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the logging service
type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	OpenSearch OpenSearchConfig `mapstructure:"opensearch"`
	Auth       AuthConfig       `mapstructure:"auth"`
	Logging    LoggingConfig    `mapstructure:"logging"`
	LogLevel   string           `mapstructure:"log_level"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// OpenSearchConfig holds OpenSearch connection configuration
type OpenSearchConfig struct {
	Address       string        `mapstructure:"address"`
	Username      string        `mapstructure:"username"`
	Password      string        `mapstructure:"password"`
	Timeout       time.Duration `mapstructure:"timeout"`
	MaxRetries    int           `mapstructure:"max_retries"`
	IndexPrefix   string        `mapstructure:"index_prefix"`
	IndexPattern  string        `mapstructure:"index_pattern"`
	LegacyPattern string        `mapstructure:"legacy_pattern"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret    string `mapstructure:"jwt_secret"`
	EnableAuth   bool   `mapstructure:"enable_auth"`
	RequiredRole string `mapstructure:"required_role"`
}

// LoggingConfig holds application logging configuration
type LoggingConfig struct {
	MaxLogLimit          int `mapstructure:"max_log_limit"`
	DefaultLogLimit      int `mapstructure:"default_log_limit"`
	DefaultBuildLogLimit int `mapstructure:"default_build_log_limit"`
	MaxLogLinesPerFile   int `mapstructure:"max_log_lines_per_file"`
}

// Load loads configuration from environment variables and defaults
func Load() (*Config, error) {
	viper.SetConfigType("env")
	viper.AutomaticEnv()

	// Set defaults
	setDefaults()

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

func setDefaults() {
	// Server defaults
	viper.SetDefault("server.port", 9097)
	viper.SetDefault("server.read_timeout", "30s")
	viper.SetDefault("server.write_timeout", "30s")
	viper.SetDefault("server.shutdown_timeout", "10s")

	// OpenSearch defaults
	viper.SetDefault("opensearch.address", "http://localhost:9200")
	viper.SetDefault("opensearch.username", "admin")
	viper.SetDefault("opensearch.password", "admin")
	viper.SetDefault("opensearch.timeout", "180s")
	viper.SetDefault("opensearch.max_retries", 3)
	viper.SetDefault("opensearch.index_prefix", "kubernetes-")
	viper.SetDefault("opensearch.index_pattern", "kubernetes-*")
	viper.SetDefault("opensearch.legacy_pattern", "choreo*")

	// Auth defaults
	viper.SetDefault("auth.enable_auth", false)
	viper.SetDefault("auth.jwt_secret", "default-secret")
	viper.SetDefault("auth.required_role", "user")

	// Logging defaults
	viper.SetDefault("logging.max_log_limit", 10000)
	viper.SetDefault("logging.default_log_limit", 100)
	viper.SetDefault("logging.default_build_log_limit", 3000)
	viper.SetDefault("logging.max_log_lines_per_file", 600000)

	// Log level
	viper.SetDefault("log_level", "info")

	// Environment variable bindings (optional - defaults are set above)
	_ = viper.BindEnv("opensearch.address", "OPENSEARCH_ADDRESS")
	_ = viper.BindEnv("opensearch.username", "OPENSEARCH_USERNAME")
	_ = viper.BindEnv("opensearch.password", "OPENSEARCH_PASSWORD")
	_ = viper.BindEnv("opensearch.timeout", "OPENSEARCH_CLIENT_TIMEOUT")
	_ = viper.BindEnv("opensearch.index_prefix", "OPENSEARCH_INDEX_PREFIX")
	_ = viper.BindEnv("auth.jwt_secret", "JWT_SECRET")
	_ = viper.BindEnv("auth.enable_auth", "ENABLE_AUTH")
	_ = viper.BindEnv("server.port", "PORT")
	_ = viper.BindEnv("log_level", "LOG_LEVEL")
	_ = viper.BindEnv("logging.max_log_limit", "MAX_LOG_LIMIT")
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
