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
		baseEnforcer.AddNamedMatchingFunc("g", "", actionMatchWrapper)
		baseEnforcer.AddNamedMatchingFunc("g2", "", actionMatchWrapper)
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
			if len(policy) != 6 {
				ce.logger.Warn("skipping malformed policy", "policy", policy, "expected", 6, "got", len(policy))
				continue
			}

			resourcePath := policy[1]
			roleName := policy[2]
			// policy[3] is role_ns, not needed for profile building
			effect := policy[4]
			// policy[5] is context

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
	if role == nil {
		return fmt.Errorf("role cannot be nil")
	}
	ce.logger.Debug("add role called", "role_name", role.Name, "namespace", role.Namespace, "actions", role.Actions)

	if role.Namespace == "" {
		return ce.addClusterRole(role)
	}
	return ce.addNamespaceRole(role)
}

// addClusterRole creates a cluster-scoped role using the default g grouping
func (ce *CasbinEnforcer) addClusterRole(role *authzcore.Role) error {
	rules := make([][]string, 0, len(role.Actions))
	for _, action := range role.Actions {
		// Cluster role: g, roleName, action
		rules = append(rules, []string{role.Name, action})
	}

	// Add all role-action mappings in a single atomic transaction using default grouping "g"
	ok, err := ce.enforcer.AddGroupingPolicies(rules)
	if err != nil {
		return fmt.Errorf("failed to add cluster role action mappings: %w", err)
	}
	// if err is nil and ok is false, some mappings already exist
	if !ok {
		return authzcore.ErrRoleAlreadyExists
	}
	return nil
}

// addNamespaceRole creates a namespace-scoped role using the g2 grouping
func (ce *CasbinEnforcer) addNamespaceRole(role *authzcore.Role) error {
	rules := make([][]string, 0, len(role.Actions))
	for _, action := range role.Actions {
		// Namespace role: g2, roleName, namespace, action
		rules = append(rules, []string{role.Name, role.Namespace, action})
	}

	// Add all role-action mappings in a single atomic transaction using named grouping "g2"
	ok, err := ce.enforcer.AddNamedGroupingPolicies("g2", rules)
	if err != nil {
		return fmt.Errorf("failed to add namespace role action mappings: %w", err)
	}
	// if err is nil and ok is false, some mappings already exist
	if !ok {
		return authzcore.ErrRoleAlreadyExists
	}
	return nil
}

// RemoveRole deletes a role identified by RoleRef
func (ce *CasbinEnforcer) RemoveRole(ctx context.Context, roleRef *authzcore.RoleRef) error {
	if roleRef == nil {
		return fmt.Errorf("role reference cannot be nil")
	}
	ce.logger.Info("remove role called", "role_name", roleRef.Name, "namespace", roleRef.Namespace)

	if roleRef.Name == "" {
		return fmt.Errorf("role name cannot be empty")
	}

	if roleRef.Namespace == "" {
		return ce.removeClusterRole(roleRef.Name)
	}
	return ce.removeNamespaceRole(roleRef.Name, roleRef.Namespace)
}

// removeClusterRole removes a cluster-scoped role from the default g grouping
func (ce *CasbinEnforcer) removeClusterRole(roleName string) error {
	// For cluster roles, check policies where role_ns = "*" (index 3)
	// Policy format: p, sub, resource, role, role_ns, eft, ctx
	policiesUsingRole, err := ce.enforcer.GetFilteredPolicy(2, roleName, "*")
	if err != nil {
		return fmt.Errorf("failed to check policies using cluster role: %w", err)
	}
	if len(policiesUsingRole) > 0 {
		ce.logger.Debug("cannot delete cluster role: role is in use",
			"role_name", roleName,
			"policy_count", len(policiesUsingRole))
		return authzcore.ErrRoleInUse
	}

	ok, err := ce.enforcer.RemoveFilteredGroupingPolicy(0, roleName)
	if err != nil {
		return fmt.Errorf("failed to remove cluster role: %w", err)
	}
	if !ok {
		return authzcore.ErrRoleNotFound
	}
	return nil
}

// removeNamespaceRole removes a namespace-scoped role from the g2 grouping
func (ce *CasbinEnforcer) removeNamespaceRole(roleName, namespace string) error {
	// For namespace roles, check policies where role_ns = namespace (index 3)
	// Policy format: p, sub, resource, role, role_ns, eft, ctx
	policiesUsingRole, err := ce.enforcer.GetFilteredPolicy(2, roleName, namespace)
	if err != nil {
		return fmt.Errorf("failed to check policies using namespace role: %w", err)
	}
	if len(policiesUsingRole) > 0 {
		ce.logger.Debug("cannot delete namespace role: role is in use",
			"role_name", roleName,
			"namespace", namespace,
			"policy_count", len(policiesUsingRole))
		return authzcore.ErrRoleInUse
	}

	ok, err := ce.enforcer.RemoveFilteredNamedGroupingPolicy("g2", 0, roleName, namespace)
	if err != nil {
		return fmt.Errorf("failed to remove namespace role: %w", err)
	}
	if !ok {
		return authzcore.ErrRoleNotFound
	}
	return nil
}

// ForceRemoveRole deletes a role and all its associated role-entitlement mappings
func (ce *CasbinEnforcer) ForceRemoveRole(ctx context.Context, roleRef *authzcore.RoleRef) error {
	if roleRef == nil {
		return fmt.Errorf("role reference cannot be nil")
	}
	ce.logger.Info("force remove role called", "role_name", roleRef.Name, "namespace", roleRef.Namespace)

	if roleRef.Name == "" {
		return fmt.Errorf("role name cannot be empty")
	}

	if roleRef.Namespace == "" {
		return ce.forceRemoveClusterRole(roleRef.Name)
	}
	return ce.forceRemoveNamespaceRole(roleRef.Name, roleRef.Namespace)
}

// forceRemoveClusterRole deletes a cluster role and all its associated role-entitlement mappings
func (ce *CasbinEnforcer) forceRemoveClusterRole(roleName string) error {
	// Check if the cluster role exists first
	roleRuleSet, err := ce.enforcer.GetFilteredGroupingPolicy(0, roleName)
	if err != nil {
		return fmt.Errorf("failed to check if cluster role exists: %w", err)
	}
	if len(roleRuleSet) == 0 {
		return authzcore.ErrRoleNotFound
	}

	// Get all p policies that reference this cluster role (role_ns = "*")
	// Policy format: p, sub, resource, role, role_ns, eft, ctx
	mappingPolicies, err := ce.enforcer.GetFilteredPolicy(2, roleName, "*")
	if err != nil {
		return fmt.Errorf("failed to get mappings using cluster role: %w", err)
	}

	if len(mappingPolicies) > 0 {
		ce.logger.Debug("removing role-entitlement mappings for cluster role",
			"role_name", roleName,
			"mapping_count", len(mappingPolicies))

		// Remove all policies that reference this role
		ok, err := ce.enforcer.RemovePolicies(mappingPolicies)
		if err != nil {
			return fmt.Errorf("failed to remove policies using cluster role: %w", err)
		}
		if !ok {
			return fmt.Errorf("failed to remove role-entitlement mappings for cluster role: %s", roleName)
		}
	}

	// Remove the role itself from g grouping
	ok, err := ce.enforcer.RemoveGroupingPolicies(roleRuleSet)
	if err != nil {
		return fmt.Errorf("failed to remove cluster role: %w", err)
	}
	if !ok {
		return fmt.Errorf("failed to remove cluster role: %s", roleName)
	}

	ce.logger.Debug("cluster role and all associated mappings removed successfully",
		"role_name", roleName,
		"removed_mappings", len(mappingPolicies))

	return nil
}

// forceRemoveNamespaceRole deletes a namespace role and all its associated role-entitlement mappings
func (ce *CasbinEnforcer) forceRemoveNamespaceRole(roleName, namespace string) error {
	// Check if the namespace role exists first (g2 format: [roleName, namespace, action])
	roleRuleSet, err := ce.enforcer.GetFilteredNamedGroupingPolicy("g2", 0, roleName, namespace)
	if err != nil {
		return fmt.Errorf("failed to check if namespace role exists: %w", err)
	}
	if len(roleRuleSet) == 0 {
		return authzcore.ErrRoleNotFound
	}

	// Get all p policies that reference this namespace role (role_ns = namespace)
	// Policy format: p, sub, resource, role, role_ns, eft, ctx
	mappingPolicies, err := ce.enforcer.GetFilteredPolicy(2, roleName, namespace)
	if err != nil {
		return fmt.Errorf("failed to get mappings using namespace role: %w", err)
	}

	if len(mappingPolicies) > 0 {
		ce.logger.Debug("removing role-entitlement mappings for namespace role",
			"role_name", roleName,
			"namespace", namespace,
			"mapping_count", len(mappingPolicies))

		// Remove all policies that reference this role
		ok, err := ce.enforcer.RemovePolicies(mappingPolicies)
		if err != nil {
			return fmt.Errorf("failed to remove policies using namespace role: %w", err)
		}
		if !ok {
			return fmt.Errorf("failed to remove role-entitlement mappings for namespace role: %s", roleName)
		}
	}

	// Remove the role itself from g2 grouping
	ok, err := ce.enforcer.RemoveNamedGroupingPolicies("g2", roleRuleSet)
	if err != nil {
		return fmt.Errorf("failed to remove namespace role: %w", err)
	}
	if !ok {
		return fmt.Errorf("failed to remove namespace role: %s", roleName)
	}

	ce.logger.Debug("namespace role and all associated mappings removed successfully",
		"role_name", roleName,
		"namespace", namespace,
		"removed_mappings", len(mappingPolicies))

	return nil
}

// GetRole retrieves a role identified by RoleRef
func (ce *CasbinEnforcer) GetRole(ctx context.Context, roleRef *authzcore.RoleRef) (*authzcore.Role, error) {
	if roleRef == nil {
		return nil, fmt.Errorf("role reference cannot be nil")
	}
	ce.logger.Debug("get role called", "role_name", roleRef.Name, "namespace", roleRef.Namespace)

	if roleRef.Name == "" {
		return nil, fmt.Errorf("role name cannot be empty")
	}

	if roleRef.Namespace == "" {
		return ce.getClusterRole(roleRef.Name)
	}
	return ce.getNamespaceRole(roleRef.Name, roleRef.Namespace)
}

// getClusterRole retrieves a cluster-scoped role from the default g grouping
func (ce *CasbinEnforcer) getClusterRole(roleName string) (*authzcore.Role, error) {
	rules, err := ce.enforcer.GetFilteredGroupingPolicy(0, roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster role: %w", err)
	}

	if len(rules) == 0 {
		return nil, authzcore.ErrRoleNotFound
	}

	actions := make([]string, 0, len(rules))
	for _, rule := range rules {
		if len(rule) != 2 {
			ce.logger.Warn("skipping invalid cluster role-action mapping", "rule", rule)
			continue
		}
		actions = append(actions, rule[1])
	}

	return &authzcore.Role{
		Name:      roleName,
		Actions:   actions,
		Namespace: "",
	}, nil
}

// getNamespaceRole retrieves a namespace-scoped role from the g2 grouping
func (ce *CasbinEnforcer) getNamespaceRole(roleName, namespace string) (*authzcore.Role, error) {
	rules, err := ce.enforcer.GetFilteredNamedGroupingPolicy("g2", 0, roleName, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace role: %w", err)
	}

	if len(rules) == 0 {
		return nil, authzcore.ErrRoleNotFound
	}

	actions := make([]string, 0, len(rules))
	for _, rule := range rules {
		if len(rule) != 3 {
			ce.logger.Warn("skipping invalid namespace role-action mapping", "rule", rule)
			continue
		}
		actions = append(actions, rule[2]) // action is at index 2 for g2
	}

	return &authzcore.Role{
		Name:      roleName,
		Actions:   actions,
		Namespace: namespace,
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

	// Get cluster roles if needed (IncludeAll or Namespace is empty)
	if filter.IncludeAll || filter.Namespace == "" {
		clusterRules, err := ce.enforcer.GetGroupingPolicy()
		if err != nil {
			return nil, fmt.Errorf("failed to get cluster roles: %w", err)
		}

		for _, rule := range clusterRules {
			if len(rule) != 2 {
				ce.logger.Warn("skipping malformed cluster role rule", "rule", rule)
				continue
			}
			key := authzcore.RoleRef{Name: rule[0], Namespace: ""}
			roleRefMap[key] = append(roleRefMap[key], rule[1])
		}
	}

	// Get namespace roles if needed (IncludeAll or specific namespace)
	if filter.IncludeAll || filter.Namespace != "" {
		namespaceRules, err := ce.enforcer.GetNamedGroupingPolicy("g2")
		if err != nil {
			return nil, fmt.Errorf("failed to get namespace roles: %w", err)
		}

		for _, rule := range namespaceRules {
			if len(rule) != 3 {
				ce.logger.Warn("skipping malformed namespace role rule", "rule", rule)
				continue
			}
			// g2 format: [roleName, namespace, action]
			roleNamespace := rule[1]

			// Skip if filtering by specific namespace and this doesn't match
			if !filter.IncludeAll && filter.Namespace != roleNamespace {
				continue
			}

			key := authzcore.RoleRef{Name: rule[0], Namespace: roleNamespace}
			roleRefMap[key] = append(roleRefMap[key], rule[2])
		}
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

	if role.Namespace == "" {
		return ce.updateClusterRole(role)
	}
	return ce.updateNamespaceRole(role)
}

// updateClusterRole updates a cluster-scoped role's actions
func (ce *CasbinEnforcer) updateClusterRole(role *authzcore.Role) error {
	existingRules, err := ce.enforcer.GetFilteredGroupingPolicy(0, role.Name)
	if err != nil {
		return fmt.Errorf("failed to get cluster role: %w", err)
	}
	if len(existingRules) == 0 {
		return authzcore.ErrRoleNotFound
	}

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
			return fmt.Errorf("failed to remove old cluster role action mappings: %w", err)
		}
		if !ok {
			return fmt.Errorf("failed to remove some cluster role action mappings for role: %s", role.Name)
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
			return fmt.Errorf("failed to add new cluster role action mappings: %w", err)
		}
		if !ok {
			return fmt.Errorf("failed to add some cluster role action mappings for role: %s", role.Name)
		}
	}

	return nil
}

// updateNamespaceRole updates a namespace-scoped role's actions
func (ce *CasbinEnforcer) updateNamespaceRole(role *authzcore.Role) error {
	// g2 format: [roleName, namespace, action]
	existingRules, err := ce.enforcer.GetFilteredNamedGroupingPolicy("g2", 0, role.Name, role.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get namespace role: %w", err)
	}
	if len(existingRules) == 0 {
		return authzcore.ErrRoleNotFound
	}

	// Extract existing actions (format: [roleName, namespace, action])
	existingActions := make([]string, 0, len(existingRules))
	for _, rule := range existingRules {
		if len(rule) == 3 {
			existingActions = append(existingActions, rule[2])
		}
	}

	// Compute diff
	added, removed := computeActionsDiff(existingActions, role.Actions)

	// Remove old actions
	if len(removed) > 0 {
		toRemove := make([][]string, 0, len(removed))
		for _, action := range removed {
			toRemove = append(toRemove, []string{role.Name, role.Namespace, action})
		}

		ok, err := ce.enforcer.RemoveNamedGroupingPolicies("g2", toRemove)
		if err != nil {
			return fmt.Errorf("failed to remove old namespace role action mappings: %w", err)
		}
		if !ok {
			return fmt.Errorf("failed to remove some namespace role action mappings for role: %s", role.Name)
		}
	}

	// Add new actions
	if len(added) > 0 {
		toAdd := make([][]string, 0, len(added))
		for _, action := range added {
			toAdd = append(toAdd, []string{role.Name, role.Namespace, action})
		}

		ok, err := ce.enforcer.AddNamedGroupingPolicies("g2", toAdd)
		if err != nil {
			return fmt.Errorf("failed to add new namespace role action mappings: %w", err)
		}
		if !ok {
			return fmt.Errorf("failed to add some namespace role action mappings for role: %s", role.Name)
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

	// Determine role_ns from RoleRef.Namespace
	// Empty namespace = cluster role (role_ns = "*")
	// Non-empty namespace = namespace role (role_ns = namespace)
	roleNs := "*"
	if mapping.RoleRef.Namespace != "" {
		roleNs = mapping.RoleRef.Namespace
	}

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
	roleNs := "*"
	if mapping.RoleRef.Namespace != "" {
		roleNs = mapping.RoleRef.Namespace
	}

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
