// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/openchoreo/openchoreo/internal/authz"
	"github.com/openchoreo/openchoreo/internal/config"
)

// AuthorizationConfig defines authorization (Casbin) settings.
type AuthorizationConfig struct {
	// Enabled enables authorization enforcement.
	Enabled bool `koanf:"enabled"`
	// DatabasePath is the path to the Casbin SQLite database.
	DatabasePath string `koanf:"database_path"`
	// SeedPoliciesFile is the path to the initial policies file.
	SeedPoliciesFile string `koanf:"seed_policies_file"`
	// CacheEnabled enables the Casbin enforcer cache.
	CacheEnabled bool `koanf:"cache_enabled"`
}

// AuthorizationDefaults returns the default authorization configuration.
func AuthorizationDefaults() AuthorizationConfig {
	return AuthorizationConfig{
		Enabled:      false,
		CacheEnabled: false,
	}
}

// Validate validates the authorization configuration.
func (c *AuthorizationConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if !c.Enabled {
		return errs // skip validation if disabled
	}

	if c.DatabasePath == "" {
		errs = append(errs, config.Required(path.Child("database_path")))
	}

	return errs
}

// ToAuthzConfig converts to the authz library config.
func (c *AuthorizationConfig) ToAuthzConfig() authz.AuthZConfig {
	return authz.AuthZConfig{
		Enabled:                  c.Enabled,
		DatabasePath:             c.DatabasePath,
		DefaultAuthzDataFilePath: c.SeedPoliciesFile,
		EnableCache:              c.CacheEnabled,
	}
}
