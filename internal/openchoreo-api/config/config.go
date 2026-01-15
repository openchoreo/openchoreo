// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/subject"
)

// ConfigLegacy represents the legacy configuration structure for openchoreo-api.
// Deprecated: Use Config with the unified configuration system instead.
type ConfigLegacy struct {
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

// LoadLegacy loads and validates the legacy configuration from the specified file path.
// Deprecated: Use NewLoader with Config instead.
func LoadLegacy(filePath string) (*ConfigLegacy, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config ConfigLegacy
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := subject.ValidateConfig(config.Security.UserTypes); err != nil {
		return nil, fmt.Errorf("invalid user type config: %w", err)
	}

	subject.SortByPriority(config.Security.UserTypes)
	return &config, nil
}

// Config is the top-level configuration for openchoreo-api.
type Config struct {
	// Server defines HTTP server settings including middleware.
	Server ServerConfig `koanf:"server"`
	// Authorization defines authorization (Casbin) settings.
	Authorization AuthorizationConfig `koanf:"authorization"`
	// OAuth defines OAuth client configurations.
	// TODO: This is IdP concern, not resource server. Discuss moving to IdP or fetching from IdP.
	OAuth OAuthConfig `koanf:"oauth"`
	// MCP defines Model Context Protocol server settings.
	MCP MCPConfig `koanf:"mcp"`
	// Logging defines logging settings.
	Logging LoggingConfig `koanf:"logging"`
}

// OAuthConfig defines OAuth client configurations.
// TODO: This is IdP concern, not resource server. Consider fetching from IdP instead.
type OAuthConfig struct {
	// Clients defines external OAuth clients that can authenticate with this API.
	Clients []ExternalClientConfig `koanf:"clients"`
}

// OAuthDefaults returns the default OAuth configuration.
func OAuthDefaults() OAuthConfig {
	return OAuthConfig{
		Clients: nil,
	}
}

// Defaults returns the default configuration.
func Defaults() Config {
	return Config{
		Server:        ServerDefaults(),
		Authorization: AuthorizationDefaults(),
		OAuth:         OAuthDefaults(),
		MCP:           MCPDefaults(),
		Logging:       LoggingDefaults(),
	}
}

// flagMappings maps CLI flag names to config paths.
var flagMappings = map[string]string{
	"server-port": "server.port",
	"log-level":   "logging.level",
}

// NewLoader creates a configuration loader with all sources loaded.
// Loading priority (highest to lowest):
//  1. CLI flags (only if explicitly set)
//  2. Environment variables (OC_API__SERVER__PORT -> server.port)
//  3. Config file (YAML)
//  4. Struct defaults
//
// If configPath is empty, no config file is loaded.
// If flags is nil, no flag overrides are applied.
func NewLoader(configPath string, flags *pflag.FlagSet) (*coreconfig.Loader, error) {
	loader := coreconfig.NewLoader("OC_API")

	if err := loader.LoadWithDefaults(Defaults(), configPath); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if flags != nil {
		if err := loader.LoadFlags(flags, flagMappings); err != nil {
			return nil, fmt.Errorf("failed to load flags: %w", err)
		}
	}

	return loader, nil
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	var errs coreconfig.ValidationErrors

	errs = append(errs, c.Server.Validate(coreconfig.NewPath("server"))...)
	errs = append(errs, c.Authorization.Validate(coreconfig.NewPath("authorization"))...)
	errs = append(errs, c.MCP.Validate(coreconfig.NewPath("mcp"))...)
	errs = append(errs, c.Logging.Validate(coreconfig.NewPath("logging"))...)

	return errs.OrNil()
}
