// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/subject"
)

// Config represents the top-level configuration structure for openchoreo-api
type Config struct {
	Security SecurityConfig `yaml:"security"`
}

// SecurityConfig represents the authorization configuration section
type SecurityConfig struct {
	UserTypes       []subject.UserTypeConfig `yaml:"user_types"`
	ExternalClients []ExternalClientConfig   `yaml:"external_clients"`
}

// ExternalClientConfig represents an external client configuration
type ExternalClientConfig struct {
	Name     string   `yaml:"name"`
	ClientID string   `yaml:"client_id"`
	Scopes   []string `yaml:"scopes"`
}

// Load loads and validates the configuration from the specified file path
func Load(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := subject.ValidateConfig(config.Security.UserTypes); err != nil {
		return nil, fmt.Errorf("invalid user type config: %w", err)
	}

	subject.SortByPriority(config.Security.UserTypes)
	return &config, nil
}

// =============================================================================
// New unified configuration system (V2)
// =============================================================================

// ConfigV2 is the top-level configuration for openchoreo-api using the unified config system.
type ConfigV2 struct {
	// Server defines HTTP server settings including middleware.
	Server ServerConfig `koanf:"server"`
	// Authorization defines authorization (Casbin) settings.
	Authorization AuthorizationConfig `koanf:"authorization"`
	// MCP defines Model Context Protocol server settings.
	MCP MCPConfig `koanf:"mcp"`
	// Logging defines logging settings.
	Logging LoggingConfig `koanf:"logging"`
}

// DefaultsV2 returns the default configuration for V2.
func DefaultsV2() ConfigV2 {
	return ConfigV2{
		Server:        ServerDefaults(),
		Authorization: AuthorizationDefaults(),
		MCP:           MCPDefaults(),
		Logging:       LoggingDefaults(),
	}
}

// LoadV2 loads configuration from file and environment variables using the unified config system.
// Environment variables use the prefix OC_API__ with double underscore for nesting.
// Example: OC_API__SERVER__PORT=9090
func LoadV2(configPath string) (*ConfigV2, error) {
	loader := coreconfig.NewLoader("OC_API")

	if err := loader.LoadWithDefaults(DefaultsV2(), configPath); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	var cfg ConfigV2
	if err := loader.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// Validate validates the V2 configuration.
func (c *ConfigV2) Validate() error {
	var errs coreconfig.ValidationErrors

	errs = append(errs, c.Server.Validate(coreconfig.NewPath("server"))...)
	errs = append(errs, c.Authorization.Validate(coreconfig.NewPath("authorization"))...)
	errs = append(errs, c.MCP.Validate(coreconfig.NewPath("mcp"))...)
	errs = append(errs, c.Logging.Validate(coreconfig.NewPath("logging"))...)

	return errs.OrNil()
}
