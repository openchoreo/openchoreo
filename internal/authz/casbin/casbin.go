// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
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
}

// CasbinConfig holds configuration for the Casbin enforcer
type CasbinConfig struct {
	DatabasePath      string // Required: Path to SQLite database path
	AuthzDataFilePath string // Optional: Path to roles YAML file (falls back to embedded if empty)
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

	// Create adapter with configured database path and authz data file
	adapter, db, err := newAdapter(config.DatabasePath, config.AuthzDataFilePath, logger)
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
		// Use roleMatchWrapper for g to handle:
		// - g: [role, action, namespace] - exact match for role/namespace, wildcard for action
		baseEnforcer.AddNamedMatchingFunc("g", "", roleActionMatchWrapper)
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
func (ce *CasbinEnforcer) GetSubjectProfile(ctx context.Context, request *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	ce.logger.Debug("get subject profile called",
		"subject_context", request.SubjectContext,
		"scope", request.Scope)

	if err := validateProfileRequest(request); err != nil {
		return nil, err
	}

	subjectCtx := request.SubjectContext
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
	resourcePath  string
	roleName      string
	roleNamespace string
	effect        string
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
			if len(policy) != 6 {
				ce.logger.Warn("skipping malformed policy", "policy", policy, "expected", 6, "got", len(policy))
				continue
			}

			resourcePath := policy[1]
			roleName := policy[2]
			roleNamespace := policy[3] // Capture role namespace
			effect := policy[4]
			// policy[5] is context

			if !isWithinScope(resourcePath, scopePath) {
				continue
			}

			filteredPolicies = append(filteredPolicies, policyInfo{
				resourcePath:  resourcePath,
				roleName:      roleName,
				roleNamespace: roleNamespace,
				effect:        effect,
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

	roleToActions := make(map[authzcore.RoleRef][]string)
	actionResources := make(map[string]map[resourceKey]bool)

	for _, p := range policies {
		roleKey := authzcore.RoleRef{
			Name:      p.roleName,
			Namespace: p.roleNamespace,
		}

		// Fetch role actions if not already cached
		if _, ok := roleToActions[roleKey]; !ok {
			roleActions, err := ce.enforcer.GetFilteredGroupingPolicy(0, p.roleName, "", p.roleNamespace)
			if err != nil {
				return nil, fmt.Errorf("failed to get actions for role '%s' in namespace '%s': %w", p.roleName, p.roleNamespace, err)
			}

			var actions []string
			for _, ra := range roleActions {
				if len(ra) != 3 {
					ce.logger.Warn("skipping malformed role-action mapping", "rule", ra)
					continue
				}
				// ra[0] = role name, ra[1] = action, ra[2] = namespace (already filtered)
				expandedActions := expandActionWildcard(ra[1], actionIdx)
				actions = append(actions, expandedActions...)
			}
			roleToActions[roleKey] = actions
		}

		// Build action resources using the cached role actions
		actions := roleToActions[roleKey]
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
	if err := ValidateCreateRoleRequest(role); err != nil {
		return err
	}

	ce.logger.Debug("add role called", "role_name", role.Name, "namespace", role.Namespace, "actions", role.Actions)

	namespace := normalizeNamespace(role.Namespace)

	rules := make([][]string, 0, len(role.Actions))
	for _, action := range role.Actions {
		// Format: g, roleName, action, namespace
		rules = append(rules, []string{role.Name, action, namespace})
	}

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

// RemoveRole deletes a role identified by RoleRef
func (ce *CasbinEnforcer) RemoveRole(ctx context.Context, roleRef *authzcore.RoleRef) error {
	if err := validateRoleRef(roleRef); err != nil {
		return err
	}

	ce.logger.Debug("remove role called", "role_name", roleRef.Name, "namespace", roleRef.Namespace)

	namespace := normalizeNamespace(roleRef.Namespace)

	// Check if role is in use by any policies
	policiesUsingRole, err := ce.enforcer.GetFilteredPolicy(2, roleRef.Name, namespace)
	if err != nil {
		return fmt.Errorf("failed to check policies using role: %w", err)
	}
	if len(policiesUsingRole) > 0 {
		ce.logger.Debug("cannot delete role: role is in use",
			"role_name", roleRef.Name,
			"namespace", namespace,
			"policy_count", len(policiesUsingRole))
		return authzcore.ErrRoleInUse
	}

	// Remove role: filter by role name (index 0) and namespace (index 2)
	ok, err := ce.enforcer.RemoveFilteredGroupingPolicy(0, roleRef.Name, "", namespace)
	if err != nil {
		return fmt.Errorf("failed to remove role: %w", err)
	}
	if !ok {
		return authzcore.ErrRoleNotFound
	}
	return nil
}

// ForceRemoveRole deletes a role and all its associated role-entitlement mappings
func (ce *CasbinEnforcer) ForceRemoveRole(ctx context.Context, roleRef *authzcore.RoleRef) error {
	if err := validateRoleRef(roleRef); err != nil {
		return err
	}
	ce.logger.Debug("force remove role called", "role_name", roleRef.Name, "namespace", roleRef.Namespace)

	namespace := normalizeNamespace(roleRef.Namespace)

	// Check if the role exists first
	roleRuleSet, err := ce.enforcer.GetFilteredGroupingPolicy(0, roleRef.Name, "", namespace)
	if err != nil {
		return fmt.Errorf("failed to check if role exists: %w", err)
	}
	if len(roleRuleSet) == 0 {
		return authzcore.ErrRoleNotFound
	}

	// Get all p policies that reference this role
	mappingPolicies, err := ce.enforcer.GetFilteredPolicy(2, roleRef.Name, namespace)
	if err != nil {
		return fmt.Errorf("failed to get mappings using role: %w", err)
	}

	if len(mappingPolicies) > 0 {
		ce.logger.Debug("removing role-entitlement mappings",
			"role_name", roleRef.Name,
			"namespace", namespace,
			"mapping_count", len(mappingPolicies))

		// Remove all policies that reference this role
		ok, err := ce.enforcer.RemovePolicies(mappingPolicies)
		if err != nil {
			return fmt.Errorf("failed to remove policies using role: %w", err)
		}
		if !ok {
			return fmt.Errorf("failed to remove role-entitlement mappings for role: %s", roleRef.Name)
		}
	}

	// Remove the role itself from g grouping
	ok, err := ce.enforcer.RemoveGroupingPolicies(roleRuleSet)
	if err != nil {
		return fmt.Errorf("failed to remove role: %w", err)
	}
	if !ok {
		return fmt.Errorf("failed to remove role: %s", roleRef.Name)
	}

	ce.logger.Debug("role and all associated mappings removed successfully",
		"role_name", roleRef.Name,
		"namespace", namespace,
		"removed_mappings", len(mappingPolicies))

	return nil
}

// GetRole retrieves a role identified by RoleRef
func (ce *CasbinEnforcer) GetRole(ctx context.Context, roleRef *authzcore.RoleRef) (*authzcore.Role, error) {
	if err := validateRoleRef(roleRef); err != nil {
		return nil, err
	}
	ce.logger.Debug("get role called", "role_name", roleRef.Name, "namespace", roleRef.Namespace)

	namespace := normalizeNamespace(roleRef.Namespace)

	rules, err := ce.enforcer.GetFilteredGroupingPolicy(0, roleRef.Name, "", namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	if len(rules) == 0 {
		return nil, authzcore.ErrRoleNotFound
	}

	actions := make([]string, 0, len(rules))
	for _, rule := range rules {
		if len(rule) != 3 {
			ce.logger.Warn("skipping invalid role-action mapping", "rule", rule)
			continue
		}
		// Verify namespace matches
		if rule[2] != namespace {
			continue
		}
		actions = append(actions, rule[1])
	}

	return &authzcore.Role{
		Name:      roleRef.Name,
		Actions:   actions,
		Namespace: roleRef.Namespace,
	}, nil
}

// ListRoles returns roles based on the provided filter
// - filter.IncludeAll=true: returns all roles (cluster + all namespaces)
// - filter.Namespace="": returns cluster-scoped roles only
// - filter.Namespace="ns1": returns namespace-scoped roles in "ns1" only
func (ce *CasbinEnforcer) ListRoles(ctx context.Context, filter *authzcore.RoleFilter) ([]*authzcore.Role, error) {
	ce.logger.Debug("list roles called", "filter", filter)

	if filter == nil {
		filter = &authzcore.RoleFilter{IncludeAll: true}
	}

	roleRefMap := make(map[authzcore.RoleRef][]string)

	var filteredRules [][]string
	var err error

	if filter.IncludeAll {
		filteredRules, err = ce.enforcer.GetGroupingPolicy()
	} else {
		namespace := normalizeNamespace(filter.Namespace)
		filteredRules, err = ce.enforcer.GetFilteredGroupingPolicy(2, namespace)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get roles: %w", err)
	}

	for _, rule := range filteredRules {
		if len(rule) != 3 {
			ce.logger.Warn("skipping malformed role rule", "rule", rule)
			continue
		}

		roleName := rule[0]
		action := rule[1]
		roleNamespace := rule[2]

		// Build the role key (empty namespace string for cluster roles)
		namespace := roleNamespace
		if roleNamespace == "*" {
			namespace = ""
		}

		key := authzcore.RoleRef{Name: roleName, Namespace: namespace}
		roleRefMap[key] = append(roleRefMap[key], action)
	}

	roles := make([]*authzcore.Role, 0, len(roleRefMap))
	for key, actions := range roleRefMap {
		roles = append(roles, &authzcore.Role{
			Name:      key.Name,
			Namespace: key.Namespace,
			Actions:   actions,
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
	if role == nil {
		return fmt.Errorf("role cannot be nil")
	}
	if len(role.Actions) == 0 {
		return fmt.Errorf("role must have at least one action")
	}
	ce.logger.Debug("update role called", "role_name", role.Name, "namespace", role.Namespace, "actions", role.Actions)

	namespace := normalizeNamespace(role.Namespace)

	existingRules, err := ce.enforcer.GetFilteredGroupingPolicy(0, role.Name, "", namespace)
	if err != nil {
		return fmt.Errorf("failed to get role: %w", err)
	}
	if len(existingRules) == 0 {
		return authzcore.ErrRoleNotFound
	}

	existingActions := make([]string, 0, len(existingRules))
	for _, rule := range existingRules {
		if len(rule) == 3 && rule[2] == namespace {
			existingActions = append(existingActions, rule[1])
		}
	}

	added, removed := computeActionsDiff(existingActions, role.Actions)

	// Remove old actions
	if len(removed) > 0 {
		toRemove := make([][]string, 0, len(removed))
		for _, action := range removed {
			toRemove = append(toRemove, []string{role.Name, action, namespace})
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
			toAdd = append(toAdd, []string{role.Name, action, namespace})
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
	if err := validateRoleEntitlementMapping(mapping); err != nil {
		return err
	}

	ce.logger.Debug("add role entitlement mapping called",
		"role", mapping.RoleRef.Name,
		"role_namespace", mapping.RoleRef.Namespace,
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

	roleNs := normalizeNamespace(mapping.RoleRef.Namespace)

	// policy: p, subject, resourcePath, role, role_ns, eft, context (6 fields)
	ok, err := ce.enforcer.AddPolicy(
		subject,
		resourcePath,
		mapping.RoleRef.Name,
		roleNs,
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
	if mapping == nil {
		return fmt.Errorf("mapping cannot be nil")
	}
	ce.logger.Debug("update role entitlement mapping called",
		"mapping_id", mapping.ID,
		"role", mapping.RoleRef.Name,
		"role_namespace", mapping.RoleRef.Namespace,
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
		return authzcore.ErrCannotModifySystemMapping
	}

	resourcePath := hierarchyToResourcePath(mapping.Hierarchy)
	subject, err := formatSubject(mapping.Entitlement.Claim, mapping.Entitlement.Value)
	if err != nil {
		return fmt.Errorf("failed to format subject: %w", err)
	}

	// Determine role_ns from RoleRef.Namespace
	roleNs := normalizeNamespace(mapping.RoleRef.Namespace)

	// Old policy uses 6-field format (V0-V5)
	oldPolicy := []string{
		existingRule.V0,
		existingRule.V1,
		existingRule.V2,
		existingRule.V3,
		existingRule.V4,
		existingRule.V5,
	}
	// New policy also uses 6-field format
	newPolicy := []string{
		subject,
		resourcePath,
		mapping.RoleRef.Name,
		roleNs,
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
		rule.V5,
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
		roleNs := rule.V3 // New field: role_ns
		effect := authzcore.PolicyEffectType(rule.V4)
		// V5 is context (currently emptyContextJSON)

		// Convert role_ns to RoleRef.Namespace
		// "*" means cluster role (empty namespace)
		roleRefNamespace := ""
		if roleNs != "*" {
			roleRefNamespace = roleNs
		}

		mappings = append(mappings, &authzcore.RoleEntitlementMapping{
			ID: rule.ID,
			RoleRef: authzcore.RoleRef{
				Name:      roleName,
				Namespace: roleRefNamespace,
			},
			Entitlement: authzcore.Entitlement{
				Claim: claim,
				Value: value,
			},
			Hierarchy: resourcePathToHierarchy(resourcePath),
			Effect:    effect,
			Context:   authzcore.Context{},
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

// TODO: once context is properly integrated, pass it to enforcer
// check performs the actual authorization check using Casbin
func (ce *CasbinEnforcer) check(request *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	resourcePath := hierarchyToResourcePath(request.Resource.Hierarchy)
	subjectCtx := request.SubjectContext

	// Validate subject context
	if subjectCtx == nil {
		return &authzcore.Decision{Decision: false}, fmt.Errorf("subject context is required")
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
		// Filter by role reference (v2 = role name, v3 = role_ns)
		if filter.RoleRef != nil && filter.RoleRef.Name != "" {
			query = query.Where("v2 = ?", filter.RoleRef.Name)

			// If namespace is specified, also filter by role_ns (v3)
			// Empty namespace means cluster role (role_ns = "*")
			// Non-empty namespace means namespace role (role_ns = namespace)
			if filter.RoleRef.Namespace != "" {
				query = query.Where("v3 = ?", filter.RoleRef.Namespace)
			}
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
