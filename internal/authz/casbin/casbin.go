// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
	userTypeDetector authzcore.Detector
}

// CasbinConfig holds configuration for the Casbin enforcer
type CasbinConfig struct {
	DatabasePath      string                     // Required: Path to SQLite database path
	RolesFilePath     string                     // Optional: Path to roles YAML file (falls back to embedded if empty)
	UserTypeConfigs   []authzcore.UserTypeConfig // Required: User type detection configuration
	EnableCache       bool                       // Optional: Enable policy cache (default: false)
	CacheTTLInSeconds int                        // Optional: Cache TTL in seconds (default: 300)
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

	if len(config.UserTypeConfigs) == 0 {
		return nil, fmt.Errorf("UserTypeConfigs is required in CasbinConfig")
	}

	// RolesFilePath is optional - will use embedded default if not provided
	if config.CacheTTLInSeconds == 0 {
		config.CacheTTLInSeconds = 300 // Default: 5 minutes
	}

	// Create user type detector
	userTypeDetector, err := authzcore.NewDetector(config.UserTypeConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to create user type detector: %w", err)
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

	var enforcer casbin.IEnforcer
	if config.EnableCache {
		syncedCachedEnforcer, err := casbin.NewSyncedCachedEnforcer(m, adapter)
		if err != nil {
			return nil, fmt.Errorf("failed to create synced cached enforcer: %w", err)
		}

		syncedCachedEnforcer.SetExpireTime(time.Duration(config.CacheTTLInSeconds) * time.Second)
		enforcer = syncedCachedEnforcer
	} else {
		enforcer, err = casbin.NewSyncedEnforcer(m, adapter)
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
		userTypeDetector: userTypeDetector,
	}

	logger.Info("casbin enforcer initialized",
		"cache_enabled", config.EnableCache,
		"user_types_count", len(config.UserTypeConfigs))

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
func (ce *CasbinEnforcer) GetSubjectProfile(ctx context.Context, request *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	ce.logger.Debug("get subject profile called",
		"subject", request.Subject,
		"scope", request.Scope)

	if err := validateProfileRequest(request); err != nil {
		return nil, err
	}

	subjectCtx, err := ce.userTypeDetector.DetectUserType(request.Subject.JwtToken)
	if err != nil {
		return nil, fmt.Errorf("failed to detect user type: %w", err)
	}

	scopePath := hierarchyToResourcePath(request.Scope)

	allConcreteActions, err := ce.actionRepository.ListConcretePublicActions()
	if err != nil {
		return nil, fmt.Errorf("failed to list concrete actions: %w", err)
	}
	actionIndex := indexActions(allConcreteActions)

	policies, err := ce.filterPoliciesBySubjectAndScope(subjectCtx, scopePath)
	if err != nil {
		return nil, err
	}

	capabilities, err := ce.buildCapabilitiesFromPolicies(policies, actionIndex)
	if err != nil {
		return nil, err
	}

	return &authzcore.UserCapabilitiesResponse{
		User:         subjectCtx,
		Capabilities: capabilities,
		GeneratedAt:  time.Now(),
	}, nil
}

type policyInfo struct {
	resourcePath string
	roleName     string
	effect       string
}

// filterPoliciesBySubjectAndScope retrieves and filters policies relevant to the subject and scope
func (ce *CasbinEnforcer) filterPoliciesBySubjectAndScope(subjectCtx *authzcore.SubjectContext, scopePath string) ([]policyInfo, error) {
	var filteredPolicies []policyInfo

	for _, entitlementValue := range subjectCtx.EntitlementValues {
		subject, err := formatSubject(subjectCtx.EntitlementClaim, entitlementValue)
		if err != nil {
			return nil, fmt.Errorf("failed to format subject: %w", err)
		}
		policies, err := ce.enforcer.GetFilteredPolicy(0, subject)
		if err != nil {
			return nil, fmt.Errorf("failed to get policies for subject '%s': %w", subject, err)
		}

		for _, policy := range policies {
			if len(policy) != 5 {
				ce.logger.Warn("skipping malformed policy", "policy", policy)
				continue
			}

			resourcePath := policy[1]
			roleName := policy[2]
			effect := policy[3]

			if !isWithinScope(resourcePath, scopePath) {
				continue
			}

			filteredPolicies = append(filteredPolicies, policyInfo{
				resourcePath: resourcePath,
				roleName:     roleName,
				effect:       effect,
			})
		}
	}

	return filteredPolicies, nil
}

// buildCapabilitiesFromPolicies constructs the capabilities map from filtered policies
func (ce *CasbinEnforcer) buildCapabilitiesFromPolicies(policies []policyInfo, actionIdx actionIndex) (map[string]*authzcore.ActionCapability, error) {
	type resourceKey struct {
		path   string
		effect string
	}

	roleToActions := make(map[string][]string)
	actionResources := make(map[string]map[resourceKey]bool)

	for _, p := range policies {
		// Fetch role actions if not already cached
		if _, ok := roleToActions[p.roleName]; !ok {
			roleActions, err := ce.enforcer.GetFilteredGroupingPolicy(0, p.roleName)
			if err != nil {
				return nil, fmt.Errorf("failed to get actions for role '%s': %w", p.roleName, err)
			}

			var actions []string
			for _, ra := range roleActions {
				if len(ra) != 2 {
					ce.logger.Warn("skipping malformed role-action mapping", "rule", ra)
					continue
				}
				expandedActions := expandActionWildcard(ra[1], actionIdx)
				actions = append(actions, expandedActions...)
			}
			roleToActions[p.roleName] = actions
		}

		// Build action resources using the cached role actions
		actions := roleToActions[p.roleName]
		for _, action := range actions {
			if actionResources[action] == nil {
				actionResources[action] = make(map[resourceKey]bool)
			}
			actionResources[action][resourceKey{path: p.resourcePath, effect: p.effect}] = true
		}
	}

	// Convert to final capabilities structure
	capabilities := make(map[string]*authzcore.ActionCapability)
	for action, resources := range actionResources {
		capability := &authzcore.ActionCapability{
			Allowed: []*authzcore.CapabilityResource{},
			Denied:  []*authzcore.CapabilityResource{},
		}

		for res := range resources {
			capRes := &authzcore.CapabilityResource{
				Path:        res.path,
				Constraints: nil,
			}
			if res.effect == string(authzcore.PolicyEffectAllow) {
				capability.Allowed = append(capability.Allowed, capRes)
			} else if res.effect == string(authzcore.PolicyEffectDeny) {
				capability.Denied = append(capability.Denied, capRes)
			}
		}

		capabilities[action] = capability
	}

	return capabilities, nil
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

	policiesUsingRole, err := ce.enforcer.GetFilteredPolicy(2, roleName)
	if err != nil {
		return fmt.Errorf("failed to check policies using role: %w", err)
	}

	if len(policiesUsingRole) > 0 {
		ce.logger.Debug("cannot delete role: role is in use by role-entitlement mappings",
			"role_name", roleName,
			"policy_count", len(policiesUsingRole))
		return authzcore.ErrRoleInUse
	}

	// No policies using this role, safe to delete
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

// ForceRemoveRole deletes a role and all its associated role-entitlement mappings
func (ce *CasbinEnforcer) ForceRemoveRole(ctx context.Context, roleName string) error {
	ce.logger.Info("force remove role called", "role_name", roleName)

	if roleName == "" {
		return fmt.Errorf("role name cannot be empty")
	}

	// Check if the role exists first
	roleRuleSet, err := ce.enforcer.GetFilteredGroupingPolicy(0, roleName)
	if err != nil {
		return fmt.Errorf("failed to check if role exists: %w", err)
	}
	if len(roleRuleSet) == 0 {
		return authzcore.ErrRoleNotFound
	}

	// Get all p mappingPolicies (role-entitlement mappings) that reference this role
	mappingPolicies, err := ce.enforcer.GetFilteredPolicy(2, roleName)
	if err != nil {
		return fmt.Errorf("failed to get mappings using role: %w", err)
	}

	if len(mappingPolicies) > 0 {
		ce.logger.Debug("removing role-entitlement mappings for role",
			"role_name", roleName,
			"mapping_count", len(mappingPolicies))

		// Remove all policies that reference this role
		ok, err := ce.enforcer.RemovePolicies(mappingPolicies)
		if err != nil {
			return fmt.Errorf("failed to remove policies using role: %w", err)
		}
		if !ok {
			return fmt.Errorf("failed to remove role-entitlement mappings for role: %s", roleName)
		}
	}

	// Remove the role itself
	ok, err := ce.enforcer.RemoveGroupingPolicies(roleRuleSet)
	if err != nil {
		return fmt.Errorf("failed to remove role: %w", err)
	}
	if !ok {
		return fmt.Errorf("failed to remove role: %s", roleName)
	}

	ce.logger.Debug("role and all associated mappings removed successfully",
		"role_name", roleName,
		"removed_mappings", len(mappingPolicies))

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

// computeActionsDiff computes the difference between existing and new actions for a role
// Returns added actions (in new but not in existing) and removed actions (in existing but not in new)
func computeActionsDiff(existingActions, newActions []string) (added, removed []string) {
	existingSet := make(map[string]struct{}, len(existingActions))
	for _, action := range existingActions {
		existingSet[action] = struct{}{}
	}

	newSet := make(map[string]struct{}, len(newActions))
	for _, action := range newActions {
		newSet[action] = struct{}{}
	}

	// Find removed actions
	for action := range existingSet {
		if _, exists := newSet[action]; !exists {
			removed = append(removed, action)
		}
	}

	// Find added actions
	for action := range newSet {
		if _, exists := existingSet[action]; !exists {
			added = append(added, action)
		}
	}

	return added, removed
}

// UpdateRole updates an existing role's actions
func (ce *CasbinEnforcer) UpdateRole(ctx context.Context, role *authzcore.Role) error {
	if len(role.Actions) == 0 {
		return fmt.Errorf("role must have at least one action")
	}
	existingRules, err := ce.enforcer.GetFilteredGroupingPolicy(0, role.Name)
	if err != nil {
		return fmt.Errorf("failed to get role: %w", err)
	}
	if len(existingRules) == 0 {
		return authzcore.ErrRoleNotFound
	}

	// Extract existing actions
	existingActions := make([]string, 0, len(existingRules))
	for _, rule := range existingRules {
		if len(rule) == 2 {
			existingActions = append(existingActions, rule[1])
		}
	}

	// Compute diff
	added, removed := computeActionsDiff(existingActions, role.Actions)

	// Remove old actions
	if len(removed) > 0 {
		toRemove := make([][]string, 0, len(removed))
		for _, action := range removed {
			toRemove = append(toRemove, []string{role.Name, action})
		}

		ok, err := ce.enforcer.RemoveGroupingPolicies(toRemove)
		if err != nil {
			return fmt.Errorf("failed to remove old role action mappings: %w", err)
		}
		if !ok {
			return fmt.Errorf("failed to remove some role action mappings for role: %s", role.Name)
		}
	}

	// Add new actions
	if len(added) > 0 {
		toAdd := make([][]string, 0, len(added))
		for _, action := range added {
			toAdd = append(toAdd, []string{role.Name, action})
		}

		ok, err := ce.enforcer.AddGroupingPolicies(toAdd)
		if err != nil {
			return fmt.Errorf("failed to add new role action mappings: %w", err)
		}
		if !ok {
			return fmt.Errorf("failed to add some role action mappings for role: %s", role.Name)
		}
	}

	return nil
}

// AddRoleEntitlementMapping creates a new role-entitlement mapping with optional conditions
func (ce *CasbinEnforcer) AddRoleEntitlementMapping(ctx context.Context, mapping *authzcore.RoleEntitlementMapping) error {
	ce.logger.Info("add role entitlement mapping called",
		"role", mapping.RoleName,
		"entitlement_claim", mapping.Entitlement.Claim,
		"entitlement_value", mapping.Entitlement.Value,
		"hierarchy", mapping.Hierarchy,
		"effect", mapping.Effect,
		"context", mapping.Context)

	resourcePath := hierarchyToResourcePath(mapping.Hierarchy)

	// Construct subject as "claim:value" for explicit claim tracking
	subject, err := formatSubject(mapping.Entitlement.Claim, mapping.Entitlement.Value)
	if err != nil {
		return fmt.Errorf("failed to format subject: %w", err)
	}

	// policy: p, subject, resourcePath, role, eft, context
	// TODO: Handle context conditions properly in the future
	ok, err := ce.enforcer.AddPolicy(
		subject,
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
		return fmt.Errorf("failed to add role entitlement mapping: %w", err)
	}

	return nil
}

// UpdateRoleEntitlementMapping updates an existing role-entitlement mapping
func (ce *CasbinEnforcer) UpdateRoleEntitlementMapping(ctx context.Context, mapping *authzcore.RoleEntitlementMapping) error {
	ce.logger.Debug("update role entitlement mapping called",
		"mapping_id", mapping.ID,
		"role", mapping.RoleName,
		"entitlement_claim", mapping.Entitlement.Claim,
		"entitlement_value", mapping.Entitlement.Value,
		"hierarchy", mapping.Hierarchy,
		"effect", mapping.Effect)

	if mapping.ID == 0 {
		return fmt.Errorf("mapping ID is required for update")
	}

	// Get existing policy to verify it exists and check if it's internal
	existingRule, err := ce.getPoliciesByID(mapping.ID)
	if err != nil {
		return err
	}

	if existingRule.IsInternal {
		return authzcore.ErrCannotDeleteSystemMapping
	}

	resourcePath := hierarchyToResourcePath(mapping.Hierarchy)
	subject, err := formatSubject(mapping.Entitlement.Claim, mapping.Entitlement.Value)
	if err != nil {
		return fmt.Errorf("failed to format subject: %w", err)
	}
	oldPolicy := []string{
		existingRule.V0,
		existingRule.V1,
		existingRule.V2,
		existingRule.V3,
		existingRule.V4,
	}
	newPolicy := []string{
		subject,
		resourcePath,
		mapping.RoleName,
		string(mapping.Effect),
		emptyContextJSON,
	}

	ok, err := ce.enforcer.UpdatePolicy(oldPolicy, newPolicy)
	if err != nil {
		return fmt.Errorf("failed to update role entitlement mapping: %w", err)
	}
	if !ok {
		return fmt.Errorf("failed to update role entitlement mapping with ID: %d", mapping.ID)
	}

	return nil
}

// RemoveRoleEntitlementMapping removes a role-entitlement mapping
func (ce *CasbinEnforcer) RemoveRoleEntitlementMapping(ctx context.Context, mappingID uint) error {
	ce.logger.Info("remove role entitlement mapping called", "mapping_id", mappingID)

	// Get policy by id from database
	rule, err := ce.getPoliciesByID(mappingID)
	if err != nil {
		return err
	}
	if rule.IsInternal {
		return authzcore.ErrCannotDeleteSystemMapping
	}

	// TODO: Handle context conditions properly in the future
	ok, err := ce.enforcer.RemovePolicy(
		rule.V0,
		rule.V1,
		rule.V2,
		rule.V3,
		rule.V4,
	)
	if !ok {
		return fmt.Errorf("failed to remove role entitlement mappingId: %d", mappingID)
	}
	if err != nil {
		return fmt.Errorf("failed to remove role entitlement mapping: %w", err)
	}

	return nil
}

// ListRoleEntitlementMappings lists role-entitlement mappings with optional filters
func (ce *CasbinEnforcer) ListRoleEntitlementMappings(ctx context.Context, filter *authzcore.RoleEntitlementMappingFilter) ([]*authzcore.RoleEntitlementMapping, error) {
	ce.logger.Debug("list role entitlement mappings called", "filter", filter)

	rules, err := ce.getFilteredPolicies(filter)
	if err != nil {
		return nil, err
	}

	mappings := make([]*authzcore.RoleEntitlementMapping, 0, len(rules))
	for _, rule := range rules {
		subject := rule.V0
		claim, value, err := parseSubject(subject)
		if err != nil {
			ce.logger.Warn("skipping malformed entitlement in mapping", "subject", subject, "error", err)
			continue
		}
		resourcePath := rule.V1
		roleName := rule.V2
		effect := authzcore.PolicyEffectType(rule.V3)
		// TODO: Handle context conditions properly in the future
		context := authzcore.Context{}

		mappings = append(mappings, &authzcore.RoleEntitlementMapping{
			ID: rule.ID,
			Entitlement: authzcore.Entitlement{
				Claim: claim,
				Value: value,
			},
			RoleName:  roleName,
			Hierarchy: resourcePathToHierarchy(resourcePath),
			Effect:    effect,
			Context:   context,
		})
	}

	return mappings, nil
}

// ListActions returns all available actions in the system
func (ce *CasbinEnforcer) ListActions(ctx context.Context) ([]string, error) {
	ce.logger.Debug("list actions called")

	actions, err := ce.actionRepository.ListPublicActions()
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

// ListUserTypes returns all configured user types in the system
func (ce *CasbinEnforcer) ListUserTypes(ctx context.Context) ([]authzcore.UserTypeInfo, error) {
	ce.logger.Debug("list user types called")

	userTypes := make([]authzcore.UserTypeInfo, len(ce.config.UserTypeConfigs))
	for i, config := range ce.config.UserTypeConfigs {
		userTypes[i] = authzcore.UserTypeInfo{
			Type:        config.Type,
			DisplayName: config.DisplayName,
			Priority:    config.Priority,
			Entitlement: authzcore.EntitlementClaimInfo{
				Name:        config.Entitlement.Claim,
				DisplayName: config.Entitlement.DisplayName,
			},
		}
	}

	return userTypes, nil
}

// formatSubject creates a subject string from claim and value
// Format: "claim:value"
func formatSubject(claim, value string) (string, error) {
	if claim == "" || value == "" {
		return "", fmt.Errorf("claim and value cannot be empty")
	}
	return fmt.Sprintf("%s:%s", claim, value), nil
}

// parseSubject extracts claim and value from a subject string
// Expected format: "claim:value"
func parseSubject(subject string) (claim, value string, err error) {
	parts := strings.SplitN(subject, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid subject format: expected 'claim:value', got '%s'", subject)
	}
	return parts[0], parts[1], nil
}

// TODO: once context is properly integrated, pass it to enforcer
// check performs the actual authorization check using Casbin
func (ce *CasbinEnforcer) check(request *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	resourcePath := hierarchyToResourcePath(request.Resource.Hierarchy)
	subject := request.Subject

	// Use the user type detector to extract subject context
	subjectCtx, err := ce.userTypeDetector.DetectUserType(subject.JwtToken)
	if err != nil {
		ce.logger.Warn("failed to detect user type", "error", err)
		return &authzcore.Decision{Decision: false}, fmt.Errorf("failed to detect user type: %w", err)
	}

	ce.logger.Debug("evaluate called",
		"subject_type", subjectCtx.Type,
		"entitlement_claim", subjectCtx.EntitlementClaim,
		"entitlements", subjectCtx.EntitlementValues,
		"resource", resourcePath,
		"action", request.Action,
		"context", request.Context)

	result := false
	decision := &authzcore.Decision{Decision: false,
		Context: &authzcore.DecisionContext{
			Reason: "no matching policies found",
		}}
	for _, entitlementValue := range subjectCtx.EntitlementValues {
		entitlement, err := formatSubject(subjectCtx.EntitlementClaim, entitlementValue)
		if err != nil {
			ce.logger.Warn("failed to format subject", "error", err)
			return &authzcore.Decision{Decision: false}, fmt.Errorf("failed to format subject: %w", err)
		}
		result, err = ce.enforcer.Enforce(
			entitlement,
			resourcePath,
			request.Action,
			emptyContextJSON,
		)
		if err != nil {
			ce.logger.Warn("enforcement failed", "error", err)
			return &authzcore.Decision{Decision: false}, fmt.Errorf("enforcement failed: %w", err)
		}
		if result {
			decision.Decision = true
			resourceInfo := fmt.Sprintf("hierarchy '%s'", resourcePath)
			if request.Resource.ID != "" {
				resourceInfo = fmt.Sprintf("%s (id: %s)", resourceInfo, request.Resource.ID)
			}
			decision.Context.Reason = fmt.Sprintf("Access granted: entitlement value '%s' authorized to perform '%s' on %s", entitlementValue, request.Action, resourceInfo)
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

// getPoliciesByID retrieves a P rule by its ID
func (ce *CasbinEnforcer) getPoliciesByID(id uint) (*CasbinRule, error) {
	var policy CasbinRule
	err := ce.db.Where("id = ? AND ptype = ?", id, "p").First(&policy).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, authzcore.ErrRolePolicyMappingNotFound
		}
		return nil, fmt.Errorf("failed to query role entitlement mapping: %w", err)
	}
	return &policy, nil
}

// getFilteredPolicies retrieves P rules from the database with optional filters applied
func (ce *CasbinEnforcer) getFilteredPolicies(filter *authzcore.RoleEntitlementMappingFilter) ([]CasbinRule, error) {
	query := ce.db.Where("ptype = ?", "p")

	if filter != nil {
		// Filter by role name (v2 column contains role name)
		if filter.RoleName != nil && *filter.RoleName != "" {
			query = query.Where("v2 = ?", *filter.RoleName)
		}

		// Filter by entitlement (v0 column contains "claim:value" subject)
		if filter.Entitlement != nil && filter.Entitlement.Claim != "" && filter.Entitlement.Value != "" {
			subject, err := formatSubject(filter.Entitlement.Claim, filter.Entitlement.Value)
			if err != nil {
				return nil, fmt.Errorf("failed to format subject: %w", err)
			}
			query = query.Where("v0 = ?", subject)
		}
	}

	var rules []CasbinRule
	if err := query.Find(&rules).Error; err != nil {
		return nil, fmt.Errorf("failed to get filtered role entitlement mappings: %w", err)
	}
	return rules, nil
}

// These var declarations enforce at compile-time that CasbinEnforcer
// implements the PDP and PAP interfaces correctly.

var (
	_ authzcore.PDP = (*CasbinEnforcer)(nil)
	_ authzcore.PAP = (*CasbinEnforcer)(nil)
)
