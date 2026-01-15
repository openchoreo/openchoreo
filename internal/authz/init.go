// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/openchoreo/openchoreo/internal/authz/casbin"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// Config holds configuration for authorization initialization.
type Config struct {
	// Enabled enables or disables authorization enforcement.
	Enabled bool
	// DatabasePath is the path to the SQLite database.
	DatabasePath string
	// RolesFile is the path to the roles YAML file containing roles and mappings (optional).
	// If empty, embedded defaults are used.
	RolesFile string
	// CacheEnabled enables the Casbin enforcer cache.
	CacheEnabled bool
	// CacheTTL is the cache time-to-live duration.
	CacheTTL time.Duration
}

// Initialize creates and returns PAP and PDP implementations based on configuration.
// When authorization is disabled, it returns a passthrough implementation that allows all operations.
func Initialize(cfg Config, logger *slog.Logger) (authzcore.PAP, authzcore.PDP, error) {
	log := logger.With("module", "authz")

	if !cfg.Enabled {
		log.Info("Authorization disabled - using passthrough implementation")
		passthroughAuthz := NewDisabledAuthorizer(logger)
		return passthroughAuthz, passthroughAuthz, nil
	}

	log.Info("Authorization enabled - initializing Casbin enforcer")

	if cfg.DatabasePath == "" {
		return nil, nil, fmt.Errorf("authz database path is required when authorization is enabled")
	}

	casbinConfig := casbin.CasbinConfig{
		DatabasePath: cfg.DatabasePath,
		RolesFile:    cfg.RolesFile,
		CacheEnabled: cfg.CacheEnabled,
		CacheTTL:     cfg.CacheTTL,
	}

	casbinAuthz, err := casbin.NewCasbinEnforcer(casbinConfig, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize Casbin enforcer: %w", err)
	}

	return casbinAuthz, casbinAuthz, nil
}
