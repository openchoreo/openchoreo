// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

//go:embed rbac_model.conf
var embeddedModel string

// CasbinEnforcer implements both PDP and PAP interfaces using Casbin with Kubernetes CRDs
type CasbinEnforcer struct {
	enforcer    casbin.IEnforcer
	k8sClient   client.Client
	ctx         context.Context
	logger      *slog.Logger
	enableCache bool
	cacheTTL    int
}

// CasbinConfig holds configuration for the Casbin enforcer.
// Policies are loaded from AuthzClusterRole, AuthzRole, AuthzClusterRoleBinding, and AuthzRoleBinding CRDs.
type CasbinConfig struct {
	K8sClient    client.Client // Required: Kubernetes client
	CacheEnabled bool          // Optional: Enable policy cache (default: false)
	CacheTTL     time.Duration // Optional: Cache TTL (default: 5m)
}

// policyInfo holds information about a filtered policy
// intermediate struct used for building user profile capabilities
type policyInfo struct {
	resourcePath  string
	roleName      string
	roleNamespace string
	effect        string
}

// NewCasbinEnforcer creates a new Casbin-based authorizer using Kubernetes CRD adapter
func NewCasbinEnforcer(ctx context.Context, config CasbinConfig, logger *slog.Logger) (*CasbinEnforcer, error) {
	if config.K8sClient == nil {
		return nil, fmt.Errorf("K8sClient is required in CasbinConfig")
	}

	// Load Casbin model from embedded string
	m, err := model.NewModelFromString(embeddedModel)
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded casbin model: %w", err)
	}

	var enforcer casbin.IEnforcer
	if config.CacheEnabled {
		syncedCachedEnforcer, err := casbin.NewSyncedCachedEnforcer(m)
		if err != nil {
			return nil, fmt.Errorf("failed to create synced cached enforcer: %w", err)
		}

		// Fallback default if CacheTTL not configured
		if config.CacheTTL == 0 {
			config.CacheTTL = 5 * time.Minute
		}
		syncedCachedEnforcer.SetExpireTime(config.CacheTTL)
		enforcer = syncedCachedEnforcer
	} else {
		enforcer, err = casbin.NewSyncedEnforcer(m)
		if err != nil {
			return nil, fmt.Errorf("failed to create synced enforcer: %w", err)
		}
	}

	// Register custom functions for the matcher
	enforcer.AddFunction("resourceMatch", resourceMatchWrapper)
	enforcer.AddFunction("ctxMatch", ctxMatchWrapper)

	// Add custom role matcher function to support action wildcards
	var baseEnforcer *casbin.Enforcer
	switch e := enforcer.(type) {
	case *casbin.SyncedEnforcer:
		baseEnforcer = e.Enforcer
	case *casbin.SyncedCachedEnforcer:
		baseEnforcer = e.SyncedEnforcer.Enforcer
	default:
		return nil, fmt.Errorf("unknown enforcer type")
	}
	if baseEnforcer != nil {
		// Use roleMatchWrapper for g to handle:
		// - g: [role, action, namespace] - exact match for role/namespace, wildcard for action
		baseEnforcer.AddNamedMatchingFunc("g", "", roleActionMatchWrapper)
	}

	// turn off auto-save to prevent policy changes via enforcer APIs
	enforcer.EnableAutoSave(false)

	// Note: Policies are NOT loaded here.
	// They will be populated by informer watchers

	ce := &CasbinEnforcer{
		enforcer:    enforcer,
		k8sClient:   config.K8sClient,
		ctx:         ctx,
		logger:      logger,
		enableCache: config.CacheEnabled,
		cacheTTL:    int(config.CacheTTL),
	}

	logger.Info("casbin enforcer initialized",
		"cache_enabled", config.CacheEnabled,
		"cache_ttl", config.CacheTTL)

	return ce, nil
}

// GetEnforcer returns the underlying Casbin enforcer for use by watchers.
// This is needed to set up informer-based policy synchronization.
func (ce *CasbinEnforcer) GetEnforcer() casbin.IEnforcer {
	return ce.enforcer
}

// These var declarations enforce at compile-time that CasbinEnforcer
// implements the PDP and PAP interfaces correctly.

var (
	_ authzcore.PDP = (*CasbinEnforcer)(nil)
	_ authzcore.PAP = (*CasbinEnforcer)(nil)
)
