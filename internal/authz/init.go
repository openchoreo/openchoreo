// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/openchoreo/openchoreo/internal/authz/casbin"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// Config holds configuration for authorization initialization.
// Policies are loaded from AuthzClusterRole, AuthzRole, AuthzClusterRoleBinding, and AuthzRoleBinding CRDs.
type Config struct {
	// Enabled enables or disables authorization enforcement.
	Enabled bool
	// CacheEnabled enables the Casbin enforcer cache.
	CacheEnabled bool
	// CacheTTL is the cache time-to-live duration.
	CacheTTL time.Duration
	// ResyncInterval is the interval for informer cache resync.
	// This triggers re-listing of resources and OnUpdate callbacks for all objects.
	// Set to 0 to disable periodic resync (watch events still work).
	ResyncInterval time.Duration
}

// Initialize creates and returns PAP and PDP implementations based on configuration.
// When authorization is disabled, it returns a passthrough implementation that allows all operations.
//
// When authorization is enabled, this function sets up informer-based watchers on the manager
// to sync policies from Kubernetes CRDs. The caller MUST:
func Initialize(ctx context.Context, mgr ctrl.Manager, cfg Config, logger *slog.Logger) (authzcore.PAP, authzcore.PDP, error) {
	log := logger.With("module", "authz")

	if !cfg.Enabled {
		log.Debug("Authorization disabled - using passthrough implementation")
		passthroughAuthz := NewDisabledAuthorizer(logger)
		return passthroughAuthz, passthroughAuthz, nil
	}

	log.Info("Authorization enabled - initializing Casbin enforcer")

	casbinConfig := casbin.CasbinConfig{
		K8sClient:    mgr.GetClient(),
		CacheEnabled: cfg.CacheEnabled,
		CacheTTL:     cfg.CacheTTL,
	}

	casbinAuthz, err := casbin.NewCasbinEnforcer(ctx, casbinConfig, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize Casbin enforcer: %w", err)
	}

	// Set up informer-based watchers to sync policies from K8s CRDs
	if err := casbin.SetupAuthzWatchers(ctx, mgr, casbinAuthz.GetEnforcer(), logger); err != nil {
		return nil, nil, fmt.Errorf("failed to set up authz watchers: %w", err)
	}

	log.Debug("Authz watchers registered - policies will be loaded when manager starts")

	return casbinAuthz, casbinAuthz, nil
}
