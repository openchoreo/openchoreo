// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"time"

	"github.com/openchoreo/openchoreo/internal/authz"
	"github.com/openchoreo/openchoreo/internal/config"
)

// AuthorizationConfig defines authorization (Casbin) settings.
type AuthorizationConfig struct {
	// Enabled enables authorization enforcement.
	Enabled bool `koanf:"enabled"`
	// DatabasePath is the path to the Casbin SQLite database.
	DatabasePath string `koanf:"database_path"`
	// RolesFile is the path to the roles YAML file (contains roles and mappings).
	RolesFile string `koanf:"roles_file"`
	// CacheEnabled enables the Casbin enforcer cache.
	CacheEnabled bool `koanf:"cache_enabled"`
	// CacheTTL is the cache time-to-live duration.
	CacheTTL time.Duration `koanf:"cache_ttl"`
}

// AuthorizationDefaults returns the default authorization configuration.
func AuthorizationDefaults() AuthorizationConfig {
	return AuthorizationConfig{
		Enabled:      false,
		CacheEnabled: false,
		CacheTTL:     5 * time.Minute,
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
func (c *AuthorizationConfig) ToAuthzConfig() authz.Config {
	return authz.Config{
		Enabled:      c.Enabled,
		DatabasePath: c.DatabasePath,
		RolesFile:    c.RolesFile,
		CacheEnabled: c.CacheEnabled,
		CacheTTL:     c.CacheTTL,
	}
}
