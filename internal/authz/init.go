// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"fmt"
	"log/slog"

	"github.com/openchoreo/openchoreo/internal/authz/casbin"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/authz/usertype"
)

// AuthZConfig holds configuration for authorization initialization
type AuthZConfig struct {
	Enabled              bool                        // Enable or disable authorization
	DatabasePath         string                      // Path to database
	DefaultRolesFilePath string                      // Path to default roles YAML file (optional)
	UserTypeConfigs      []usertype.UserTypeConfig   // Required: User type detection configuration
	EnableCache          bool                        // Enable authz caching
}

// Initialize creates and returns PAP and PDP implementations based on configuration.
// When authorization is disabled, it returns a passthrough implementation that allows all operations.
func Initialize(config AuthZConfig, logger *slog.Logger) (authzcore.PAP, authzcore.PDP, error) {
	if !config.Enabled {
		logger.Info("Authorization disabled - using passthrough implementation")
		passthroughAuthz := NewDisabledAuthorizer(logger.With("component", "authz.passthrough"))
		return passthroughAuthz, passthroughAuthz, nil
	}

	// Authorization enabled - initialize Casbin enforcer
	logger.Info("Authorization enabled - initializing Casbin enforcer")

	if config.DatabasePath == "" {
		return nil, nil, fmt.Errorf("authz database path is required when authorization is enabled")
	}

	if len(config.UserTypeConfigs) == 0 {
		return nil, nil, fmt.Errorf("authz user type configs are required when authorization is enabled")
	}

	casbinConfig := casbin.CasbinConfig{
		DatabasePath:    config.DatabasePath,
		RolesFilePath:   config.DefaultRolesFilePath, // Can be empty, will use embedded default
		UserTypeConfigs: config.UserTypeConfigs,
		EnableCache:     config.EnableCache,
	}

	casbinAuthz, err := casbin.NewCasbinEnforcer(casbinConfig, logger.With("component", "authz.casbin"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize Casbin enforcer: %w", err)
	}

	return casbinAuthz, casbinAuthz, nil
}
