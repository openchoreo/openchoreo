// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"gorm.io/gorm"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

//go:embed rbac_model.conf
var embeddedModel string

// CasbinEnforcer implements both PDP and PAP interfaces using Casbin
type CasbinEnforcer struct {
	enforcer         casbin.IEnforcer
	config           CasbinConfig
	logger           *slog.Logger
	actionRepository *ActionRepository
	db               *gorm.DB
}

// CasbinConfig holds configuration for the Casbin enforcer
type CasbinConfig struct {
	DatabasePath      string // Required: Path to SQLite database path
	RolesFilePath     string // Optional: Path to roles YAML file (falls back to embedded if empty)
	EnableCache       bool   // Optional: Enable policy cache (default: false)
	CacheTTLInSeconds int    // Optional: Cache TTL in seconds (default: 300)
}

const (
	// emptyContextJSON represents an empty context used when no contextual conditions are applied
	// TODO: Replace with proper context handling when context matching is implemented
	emptyContextJSON = "{}"
)

// NewCasbinEnforcer creates a new Casbin-based authorizer
func NewCasbinEnforcer(config CasbinConfig, logger *slog.Logger) (*CasbinEnforcer, error) {
	if config.DatabasePath == "" {
		return nil, fmt.Errorf("DatabasePath is required in CasbinConfig")
	}

	// RolesFilePath is optional - will use embedded default if not provided
	if config.CacheTTLInSeconds == 0 {
		config.CacheTTLInSeconds = 300 // Default: 5 minutes
	}

	// Load Casbin model from embedded string
	m, err := model.NewModelFromString(embeddedModel)
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded casbin model: %w", err)
	}

	// Create adapter with configured database path and roles file
	adapter, db, err := newAdapter(config.DatabasePath, config.RolesFilePath, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create casbin adapter: %w", err)
	}

	// Create action repository
	actionRepo := NewActionRepository(db)

	// Create enforcer with or without caching
	var enforcer casbin.IEnforcer
	if config.EnableCache {
		enforcer, err = casbin.NewCachedEnforcer(m, adapter)
		if err != nil {
			return nil, fmt.Errorf("failed to create casbin cached enforcer: %w", err)
		}
	} else {
		enforcer, err = casbin.NewEnforcer(m, adapter)
		if err != nil {
			return nil, fmt.Errorf("failed to create casbin enforcer: %w", err)
		}
	}

	// Register custom functions for the matcher
	enforcer.AddFunction("resourceMatch", resourceMatchWrapper)
	enforcer.AddFunction("ctxMatch", ctxMatchWrapper)

	// Add custom role matcher function to support action wildcards
	var baseEnforcer *casbin.Enforcer
	switch e := enforcer.(type) {
	case *casbin.Enforcer:
		baseEnforcer = e
	case *casbin.CachedEnforcer:
		baseEnforcer = e.Enforcer
	default:
		return nil, fmt.Errorf("unknown enforcer type")
	}
	if baseEnforcer != nil {
		baseEnforcer.AddNamedMatchingFunc("g", "", actionMatchWrapper)
	}
	// Load policies from database
	if err := enforcer.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("failed to load policies: %w", err)
	}

	ce := &CasbinEnforcer{
		enforcer:         enforcer,
		config:           config,
		logger:           logger,
		actionRepository: actionRepo,
		db:               db,
	}

	logger.Info("casbin enforcer initialized",
		"cache_enabled", config.EnableCache)

	return ce, nil
}

// ============================================================================
// PDP Implementation
// ============================================================================

// Evaluate evaluates a single authorization request and returns a decision
func (ce *CasbinEnforcer) Evaluate(ctx context.Context, request *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	err := validateEvaluateRequest(request)
	if err != nil {
		return &authzcore.Decision{Decision: false}, err
	}
	return ce.check(request)
}

// BatchEvaluate evaluates multiple authorization requests and returns corresponding decisions
// NOTE: if needed, can be enhanced to do in parallel
func (ce *CasbinEnforcer) BatchEvaluate(ctx context.Context, request *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	err := validateBatchEvaluateRequest(request)
	if err != nil {
		return &authzcore.BatchEvaluateResponse{}, err
	}

	decisions := make([]authzcore.Decision, len(request.Requests))
	for i, req := range request.Requests {
		// Check for context cancellation
		if ctx.Err() != nil {
			return &authzcore.BatchEvaluateResponse{}, ctx.Err()
		}
		decision, err := ce.check(&req)
		if err != nil {
			return &authzcore.BatchEvaluateResponse{}, fmt.Errorf("batch evaluate failed at index %d: %w", i, err)
		}
		decisions[i] = *decision
	}

	return &authzcore.BatchEvaluateResponse{
		Decisions: decisions,
	}, nil
}

// GetSubjectProfile retrieves the authorization profile for a given subject
func (ce *CasbinEnforcer) GetSubjectProfile(ctx context.Context, request *authzcore.ProfileRequest) (*authzcore.SubjectProfile, error) {
	// TODO: Implement subject profile retrieval logic
	ce.logger.Debug("get subject profile called",
		"subject", request.Subject,
		"scope", request.Scope)

	// Placeholder implementation
	profile := &authzcore.SubjectProfile{
		Hierarchy: authzcore.ProfileResourceNode{
			Type:     "organization",
			ID:       request.Scope.Organization,
			Actions:  []string{},
			Children: []authzcore.ProfileResourceNode{},
		},
	}

	return profile, nil
}

// ============================================================================
// PAP Implementation
// ============================================================================

// AddRole creates a new role with the specified name and actions
func (ce *CasbinEnforcer) AddRole(ctx context.Context, role *authzcore.Role) error {
	ce.logger.Info("add role called", "role_name", role.Name, "actions", role.Actions)

	rules := make([][]string, 0, len(role.Actions))
	for _, action := range role.Actions {
		rules = append(rules, []string{role.Name, action})
	}

	// Add all role-action mappings in a single atomic transaction
	ok, err := ce.enforcer.AddGroupingPolicies(rules)
	if err != nil {
		return fmt.Errorf("failed to add role action mappings: %w", err)
	}
	// if err is nil and ok is false, some mappings already exist
	if !ok {
		return authzcore.ErrRoleAlreadyExists
	}
	return nil
}

// RemoveRole deletes a role by name
func (ce *CasbinEnforcer) RemoveRole(ctx context.Context, roleName string) error {
	ce.logger.Info("remove role called", "role_name", roleName)

	if roleName == "" {
		return fmt.Errorf("role name cannot be empty")
	}

	ok, err := ce.enforcer.RemoveFilteredGroupingPolicy(0, roleName)
	if err != nil {
		return fmt.Errorf("failed to remove role: %w", err)
	}
	// if err is nil and ok is false, role did not exist
	if !ok {
		return authzcore.ErrRoleNotFound
	}

	return nil
}

// GetRole retrieves a role by name
func (ce *CasbinEnforcer) GetRole(ctx context.Context, roleName string) (*authzcore.Role, error) {
	ce.logger.Debug("get role called", "role_name", roleName)

	if roleName == "" {
		return nil, fmt.Errorf("role name cannot be empty")
	}

	rules, err := ce.enforcer.GetFilteredGroupingPolicy(0, roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	if len(rules) == 0 {
		return nil, fmt.Errorf("role not found: %s", roleName)
	}

	actions := make([]string, 0, len(rules))
	for _, rule := range rules {
		if len(rule) != 2 {
			ce.logger.Warn("skipping invalid role-action mapping", "rule", rule)
			continue
		}
		actions = append(actions, rule[1])
	}

	return &authzcore.Role{
		Name:    roleName,
		Actions: actions,
	}, nil
}

// ListRoles returns all defined roles
func (ce *CasbinEnforcer) ListRoles(ctx context.Context) ([]*authzcore.Role, error) {
	ce.logger.Debug("list roles called")

	rules, err := ce.enforcer.GetGroupingPolicy()
	if err != nil {
		return nil, fmt.Errorf("failed to get roles: %w", err)
	}

	roleMap := make(map[string][]string)
	for _, rule := range rules {
		if len(rule) != 2 {
			ce.logger.Warn("skipping malformed role rule", "rule", rule)
			continue
		}
		roleName := rule[0]
		action := rule[1]
		roleMap[roleName] = append(roleMap[roleName], action)
	}

	roles := make([]*authzcore.Role, 0, len(roleMap))
	for roleName, actions := range roleMap {
		roles = append(roles, &authzcore.Role{
			Name:    roleName,
			Actions: actions,
		})
	}

	return roles, nil
}

// AddRoleEntitlementMapping creates a new role-principal mapping with optional conditions
func (ce *CasbinEnforcer) AddRoleEntitlementMapping(ctx context.Context, mapping *authzcore.RoleEntitlementMapping) error {
	ce.logger.Info("add role principal mapping called",
		"role", mapping.RoleName,
		"principal", mapping.EntitlementValue,
		"hierarchy", mapping.Hierarchy,
		"effect", mapping.Effect,
		"context", mapping.Context)

	resourcePath := hierarchyToResourcePath(mapping.Hierarchy)

	// policy: p, subject, resourcePath, role, eft, context
	// TODO: Handle context conditions properly in the future
	ok, err := ce.enforcer.AddPolicy(
		mapping.EntitlementValue,
		resourcePath,
		mapping.RoleName,
		string(mapping.Effect),
		emptyContextJSON,
	)
	// if err is nil and ok is false, some mappings already exist
	if !ok {
		return authzcore.ErrRolePolicyMappingAlreadyExists
	}

	if err != nil {
		return fmt.Errorf("failed to add role principal mapping: %w", err)
	}

	return nil
}

// RemoveRoleEntitlementMapping removes a role-principal mapping
func (ce *CasbinEnforcer) RemoveRoleEntitlementMapping(ctx context.Context, mapping *authzcore.RoleEntitlementMapping) error {
	ce.logger.Info("remove role principal mapping called",
		"role", mapping.RoleName,
		"principal", mapping.EntitlementValue,
		"hierarchy", mapping.Hierarchy,
		"effect", mapping.Effect,
		"context", mapping.Context,
	)

	resourcePath := hierarchyToResourcePath(mapping.Hierarchy)
	// TODO: Handle context conditions properly in the future
	ok, err := ce.enforcer.RemovePolicy(
		mapping.EntitlementValue,
		resourcePath,
		mapping.RoleName,
		string(mapping.Effect),
		emptyContextJSON,
	)
	// if err is nil and ok is false, mapping did not exist
	if !ok {
		return authzcore.ErrRolePolicyMappingNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to remove role principal mapping: %w", err)
	}

	return nil
}

// ListRoleEntitlementMappings lists all role-principal mappings
func (ce *CasbinEnforcer) ListRoleEntitlementMappings(ctx context.Context) ([]*authzcore.RoleEntitlementMapping, error) {
	ce.logger.Debug("list role principal mappings called")

	rules, err := ce.enforcer.GetPolicy()
	if err != nil {
		return nil, fmt.Errorf("failed to get role principal mappings: %w", err)
	}

	mappings := make([]*authzcore.RoleEntitlementMapping, 0, len(rules))
	for _, rule := range rules {
		if len(rule) < 5 {
			ce.logger.Warn("skipping malformed role-principal mapping", "rule", rule)
			continue
		}
		principal := rule[0]
		resourcePath := rule[1]
		roleName := rule[2]
		effect := authzcore.PolicyEffectType(rule[3])
		// TODO: Handle context conditions properly in the future
		context := authzcore.Context{}

		mappings = append(mappings, &authzcore.RoleEntitlementMapping{
			EntitlementValue: principal,
			RoleName:         roleName,
			Hierarchy:        resourcePathToHierarchy(resourcePath),
			Effect:           effect,
			Context:          context,
		})
	}

	return mappings, nil
}

// GetEntitlementMappings retrieves all role mappings for a specific principal
func (ce *CasbinEnforcer) GetEntitlementMappings(ctx context.Context, principal string) ([]*authzcore.RoleEntitlementMapping, error) {
	// TODO: Implement principal mappings retrieval logic
	ce.logger.Debug("get principal mappings called", "principal", principal)

	// Placeholder implementation
	mappings := []*authzcore.RoleEntitlementMapping{}

	return mappings, nil
}

// GetRoleMappings retrieves all principal mappings for a specific role
func (ce *CasbinEnforcer) GetRoleMappings(ctx context.Context, roleName string) ([]*authzcore.RoleEntitlementMapping, error) {
	// TODO: Implement role mappings retrieval logic
	ce.logger.Debug("get role mappings called", "role_name", roleName)

	// Placeholder implementation
	mappings := []*authzcore.RoleEntitlementMapping{}

	return mappings, nil
}

// ListActions returns all available actions in the system
func (ce *CasbinEnforcer) ListActions(ctx context.Context) ([]string, error) {
	ce.logger.Debug("list actions called")

	actions, err := ce.actionRepository.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list actions: %w", err)
	}

	// Convert []Action to []string
	result := make([]string, len(actions))
	for i, action := range actions {
		result[i] = action.Action
	}

	return result, nil
}

// TODO: once context is properly integrated, pass it to enforcer

func (ce *CasbinEnforcer) check(request *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	resourcePath := hierarchyToResourcePath(request.Resource.Hierarchy)
	subject := request.Subject
	subjectCtx, err := populateSubjectClaims(&subject)
	if err != nil {
		return &authzcore.Decision{Decision: false}, fmt.Errorf("failed to extract subject claims: %w", err)
	}

	ce.logger.Debug("evaluate called",
		"subject", subjectCtx.EntitlementValues,
		"resource", resourcePath,
		"action", request.Action,
		"context", request.Context)

	result := false
	decision := &authzcore.Decision{Decision: false,
		Context: &authzcore.DecisionContext{
			Reason: "no matching policies found",
		}}
	for _, entitlementValue := range subjectCtx.EntitlementValues {
		result, err = ce.enforcer.Enforce(
			entitlementValue,
			resourcePath,
			request.Action,
			emptyContextJSON,
		)
		if err != nil {
			return &authzcore.Decision{Decision: false}, fmt.Errorf("enforcement failed: %w", err)
		}
		if result {
			decision.Decision = true
			resourceInfo := fmt.Sprintf("hierarchy '%s'", resourcePath)
			if request.Resource.ID != "" {
				resourceInfo = fmt.Sprintf("%s (id: %s)", resourceInfo, request.Resource.ID)
			}
			decision.Context.Reason = fmt.Sprintf("Access granted: principal '%s' authorized to perform '%s' on %s", entitlementValue, request.Action, resourceInfo)
			break
		}
	}
	return decision, nil
}

// Close closes the database connection and cleans up resources
func (ce *CasbinEnforcer) Close() error {
	if ce.db != nil {
		sqlDB, err := ce.db.DB()
		if err != nil {
			return fmt.Errorf("failed to get underlying database connection: %w", err)
		}
		if err := sqlDB.Close(); err != nil {
			return fmt.Errorf("failed to close database connection: %w", err)
		}
		ce.logger.Info("casbin database connection closed")
	}
	return nil
}

// These var declarations enforce at compile-time that CasbinEnforcer
// implements the PDP and PAP interfaces correctly.

var (
	_ authzcore.PDP = (*CasbinEnforcer)(nil)
	_ authzcore.PAP = (*CasbinEnforcer)(nil)
)
