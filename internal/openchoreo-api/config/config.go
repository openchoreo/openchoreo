// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	"github.com/spf13/pflag"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

// Config is the top-level configuration for openchoreo-api.
type Config struct {
	// Server defines HTTP server settings.
	Server ServerConfig `koanf:"server"`
	// Security defines authentication and authorization settings.
	Security SecurityConfig `koanf:"security"`
	// Identity defines identity provider settings.
	Identity IdentityConfig `koanf:"identity"`
	// MCP defines Model Context Protocol server settings.
	MCP MCPConfig `koanf:"mcp"`
	// Logging defines logging settings.
	Logging LoggingConfig `koanf:"logging"`
}

// Defaults returns the default configuration.
func Defaults() Config {
	return Config{
		Server:   ServerDefaults(),
		Security: SecurityDefaults(),
		Identity: IdentityDefaults(),
		MCP:      MCPDefaults(),
		Logging:  LoggingDefaults(),
	}
}

// flagMappings maps CLI flag names to config paths.
var flagMappings = map[string]string{
	"server-bind-address": "server.bind_address",
	"server-port":         "server.port",
	"log-level":           "logging.level",
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
	errs = append(errs, c.Security.Validate(coreconfig.NewPath("security"))...)
	errs = append(errs, c.Identity.Validate(coreconfig.NewPath("identity"))...)
	errs = append(errs, c.MCP.Validate(coreconfig.NewPath("mcp"))...)
	errs = append(errs, c.Logging.Validate(coreconfig.NewPath("logging"))...)

	return errs.OrNil()
}
