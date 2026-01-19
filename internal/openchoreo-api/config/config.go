// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	"github.com/spf13/pflag"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

// ExternalClientConfig represents an external client configuration
type ExternalClientConfig struct {
	Name     string   `koanf:"name"`
	ClientID string   `koanf:"client_id"`
	Scopes   []string `koanf:"scopes"`
}

// Config is the top-level configuration for openchoreo-api.
type Config struct {
	// Server defines HTTP server settings including middleware.
	Server ServerConfig `koanf:"server"`
	// Authorization defines authorization (Casbin) settings.
	Authorization AuthorizationConfig `koanf:"authorization"`
	// OAuth defines OAuth client configurations.
	OAuth OAuthConfig `koanf:"oauth"`
	// MCP defines Model Context Protocol server settings.
	MCP MCPConfig `koanf:"mcp"`
	// Logging defines logging settings.
	Logging LoggingConfig `koanf:"logging"`
}

// OAuthConfig defines OAuth client configurations.
type OAuthConfig struct {
	// OIDC defines OIDC endpoint URLs for client discovery.
	OIDC OIDCConfig `koanf:"oidc"`
	// Clients defines external OAuth clients that can authenticate with this API.
	Clients []ExternalClientConfig `koanf:"clients"`
}

// OIDCConfig defines OIDC endpoint URLs for client discovery.
type OIDCConfig struct {
	// AuthorizationURL is the OAuth authorization endpoint URL.
	AuthorizationURL string `koanf:"authorization_url"`
	// TokenURL is the OAuth token endpoint URL.
	TokenURL string `koanf:"token_url"`
}

// OIDCDefaults returns the default OIDC configuration.
func OIDCDefaults() OIDCConfig {
	return OIDCConfig{
		AuthorizationURL: "http://sts.openchoreo.localhost/oauth2/authorize",
		TokenURL:         "http://sts.openchoreo.localhost/oauth2/token",
	}
}

// OAuthDefaults returns the default OAuth configuration.
func OAuthDefaults() OAuthConfig {
	return OAuthConfig{
		OIDC:    OIDCDefaults(),
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
