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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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

// // GetSubjectProfile retrieves the authorization profile for a given subject
func (ce *CasbinEnforcer) GetSubjectProfile(ctx context.Context, request *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	ce.logger.Debug("get subject profile called",
		"subject_context", request.SubjectContext,
		"scope", request.Scope)

	if err := validateProfileRequest(request); err != nil {
		return nil, err
	}

	subjectCtx := request.SubjectContext
	scopePath := resourceHierarchyToPath(request.Scope)

	allConcreteActions := authzcore.ConcretePublicActions()

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
// For K8s implementation, creates AuthzRole (namespaced) or AuthzClusterRole (cluster-scoped) CRD
func (ce *CasbinEnforcer) AddRole(ctx context.Context, role *authzcore.Role) error {
	if err := ValidateCreateRoleRequest(role); err != nil {
		return err
	}
	ce.logger.Debug("add role called", "role_name", role.Name, "namespace", role.Namespace, "actions", role.Actions)

	if isClusterScoped(role.Namespace) {
		return ce.createClusterRole(ctx, role)
	}
	return ce.createNamespacedRole(ctx, role)

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

	if isClusterScoped(roleRef.Namespace) {
		return ce.deleteClusterRole(ctx, roleRef)
	}
	return ce.deleteNamespacedRole(ctx, roleRef)
}

// ForceRemoveRole deletes a role and all its associated role-entitlement mappings
func (ce *CasbinEnforcer) ForceRemoveRole(ctx context.Context, roleRef *authzcore.RoleRef) error {
	if err := validateRoleRef(roleRef); err != nil {
		return err
	}
	ce.logger.Debug("force remove role called", "role_name", roleRef.Name, "namespace", roleRef.Namespace)

	// First delete all bindings that reference this role
	if err := ce.deleteRoleBindingsForRole(ctx, roleRef); err != nil {
		return fmt.Errorf("failed to delete role bindings: %w", err)
	}

	// Then delete the role itself
	if isClusterScoped(roleRef.Namespace) {
		return ce.deleteClusterRole(ctx, roleRef)
	}
	return ce.deleteNamespacedRole(ctx, roleRef)
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

// UpdateRole updates an existing role's actions
func (ce *CasbinEnforcer) UpdateRole(ctx context.Context, role *authzcore.Role) error {
	if role == nil {
		return fmt.Errorf("role cannot be nil")
	}
	if len(role.Actions) == 0 {
		return fmt.Errorf("role must have at least one action")
	}
	ce.logger.Debug("update role called", "role_name", role.Name, "namespace", role.Namespace, "actions", role.Actions)

	if isClusterScoped(role.Namespace) {
		return ce.updateClusterRoleActions(ctx, role)
	}
	return ce.updateNamespacedRoleActions(ctx, role)
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

	bindingObj := ce.buildBindingFromMapping(mapping)
	if err := ce.k8sClient.Create(ctx, bindingObj); err != nil {
		return fmt.Errorf("failed to create binding: %w", err)
	}

	ce.logger.Debug("created binding", "name", mapping.Name, "namespace", mapping.Hierarchy.Namespace)
	return nil
}

// UpdateRoleEntitlementMapping updates an existing role-entitlement mapping
func (ce *CasbinEnforcer) UpdateRoleEntitlementMapping(ctx context.Context, mapping *authzcore.RoleEntitlementMapping) error {
	if err := validateRoleEntitlementMapping(mapping); err != nil {
		return err
	}

	ce.logger.Debug("update role entitlement mapping called",
		"role", mapping.RoleRef.Name,
		"role_namespace", mapping.RoleRef.Namespace,
		"binding_name", mapping.Name,
		"hierarchy_namespace", mapping.Hierarchy.Namespace)

	// Check if it's a cluster binding or namespaced binding based on hierarchy namespace
	if isClusterScoped(mapping.Hierarchy.Namespace) {
		return ce.updateClusterRoleBinding(ctx, mapping)
	}
	return ce.updateNamespacedRoleBinding(ctx, mapping)
}

// RemoveRoleEntitlementMapping removes a role-entitlement mapping
func (ce *CasbinEnforcer) RemoveRoleEntitlementMapping(ctx context.Context, mappingRef *authzcore.MappingRef) error {
	if mappingRef == nil {
		return fmt.Errorf("mappingRef cannot be nil")
	}

	ce.logger.Debug("remove role entitlement mapping called",
		"name", mappingRef.Name,
		"namespace", mappingRef.Namespace)

	// If namespace is empty, it's a cluster-scoped binding
	if mappingRef.Namespace == "" {
		clusterBinding := &openchoreov1alpha1.AuthzClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: mappingRef.Name,
			},
		}
		if err := ce.k8sClient.Delete(ctx, clusterBinding); err != nil {
			return fmt.Errorf("failed to delete AuthzClusterRoleBinding: %w", err)
		}
		ce.logger.Debug("deleted AuthzClusterRoleBinding", "name", mappingRef.Name)
		return nil
	}

	// Otherwise, it's a namespaced binding
	roleBinding := &openchoreov1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mappingRef.Name,
			Namespace: mappingRef.Namespace,
		},
	}
	if err := ce.k8sClient.Delete(ctx, roleBinding); err != nil {
		return fmt.Errorf("failed to delete AuthzRoleBinding: %w", err)
	}
	ce.logger.Debug("deleted AuthzRoleBinding", "name", mappingRef.Name, "namespace", mappingRef.Namespace)
	return nil
}

// ListRoleEntitlementMappings lists role-entitlement mappings with optional filters
func (ce *CasbinEnforcer) ListRoleEntitlementMappings(ctx context.Context, filter *authzcore.RoleEntitlementMappingFilter) ([]*authzcore.RoleEntitlementMapping, error) {
	ce.logger.Debug("list role entitlement mappings called", "filter", filter)

	var subject, roleName, roleNamespace, effect string
	var policies [][]string
	var err error

	if filter != nil {
		if filter.Entitlement != nil {
			// Format subject as "claim:value"
			subject, err = formatSubject(filter.Entitlement.Claim, filter.Entitlement.Value)
			if err != nil {
				return nil, fmt.Errorf("failed to format subject: %w", err)
			}
		}
		if filter.RoleRef != nil {
			roleName = filter.RoleRef.Name
			roleNamespace = normalizeNamespace(filter.RoleRef.Namespace)
		}

		if filter.Effect != nil {
			effect = string(*filter.Effect)
		}
	}

	// Policy format: [subject, resource, role, namespace, effect, context]
	// Filter starting from index 0: subject, resource (skip with ""), role, namespace
	policies, err = ce.enforcer.GetFilteredPolicy(0, subject, "", roleName, roleNamespace, effect)
	if err != nil {
		return nil, fmt.Errorf("failed to get policies from enforcer: %w", err)
	}

	var mappings []*authzcore.RoleEntitlementMapping

	for _, policy := range policies {
		if len(policy) != 6 {
			ce.logger.Warn("skipping malformed policy", "policy", policy, "expected", 6, "got", len(policy))
			continue
		}

		// Policy format: [subject, resource, role, namespace, effect, context]
		policySubject := policy[0]
		resourcePath := policy[1]
		policyRole := policy[2]
		policyNamespace := policy[3]
		policyEffect := policy[4]
		// policy[5] is context

		// Parse subject into entitlement claim and value
		entitlementClaim, entitlementValue, err := parseSubject(policySubject)
		if err != nil {
			ce.logger.Warn("skipping policy with invalid subject", "subject", policySubject, "error", err)
			continue
		}

		// Parse resource path into hierarchy
		hierarchy := resourcePathToHierarchy(resourcePath)

		// Determine role namespace (empty string for cluster roles, indicated by "*")
		roleNs := policyNamespace
		if policyNamespace == "*" {
			roleNs = ""
		}

		mapping := &authzcore.RoleEntitlementMapping{
			RoleRef: authzcore.RoleRef{
				Name:      policyRole,
				Namespace: roleNs,
			},
			Entitlement: authzcore.Entitlement{
				Claim: entitlementClaim,
				Value: entitlementValue,
			},
			Hierarchy: hierarchy,
			Effect:    authzcore.PolicyEffectType(policyEffect),
		}

		mappings = append(mappings, mapping)
	}

	return mappings, nil
}

// ListActions returns all available actions in the system
func (ce *CasbinEnforcer) ListActions(ctx context.Context) ([]string, error) {
	actions := authzcore.PublicActions()
	names := make([]string, len(actions))
	for i, action := range actions {
		names[i] = action.Name
	}
	return names, nil
}

// updateClusterRoleBinding updates an existing AuthzClusterRoleBinding
func (ce *CasbinEnforcer) updateClusterRoleBinding(ctx context.Context, mapping *authzcore.RoleEntitlementMapping) error {
	existingBinding := &openchoreov1alpha1.AuthzClusterRoleBinding{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: mapping.Name}, existingBinding); err != nil {
		return fmt.Errorf("cluster role binding not found: %w", err)
	}

	// Determine the role kind based on whether the role is cluster-scoped or not
	roleKind := CRDTypeAuthzRole
	if isClusterScoped(mapping.RoleRef.Namespace) {
		roleKind = CRDTypeAuthzClusterRole
	}

	// Update spec fields directly on the existing object
	existingBinding.Spec.Entitlement.Claim = mapping.Entitlement.Claim
	existingBinding.Spec.Entitlement.Value = mapping.Entitlement.Value
	existingBinding.Spec.RoleRef.Kind = roleKind
	existingBinding.Spec.RoleRef.Name = mapping.RoleRef.Name
	existingBinding.Spec.Effect = openchoreov1alpha1.EffectType(mapping.Effect)

	if err := ce.k8sClient.Update(ctx, existingBinding); err != nil {
		return fmt.Errorf("failed to update AuthzClusterRoleBinding: %w", err)
	}

	ce.logger.Debug("updated AuthzClusterRoleBinding", "name", mapping.Name)
	return nil
}

// updateNamespacedRoleBinding updates an existing AuthzRoleBinding
func (ce *CasbinEnforcer) updateNamespacedRoleBinding(ctx context.Context, mapping *authzcore.RoleEntitlementMapping) error {
	existingBinding := &openchoreov1alpha1.AuthzRoleBinding{}
	key := client.ObjectKey{Name: mapping.Name, Namespace: mapping.Hierarchy.Namespace}
	if err := ce.k8sClient.Get(ctx, key, existingBinding); err != nil {
		return fmt.Errorf("role binding not found: %w", err)
	}

	// Determine the role kind based on whether the role is cluster-scoped or not
	roleKind := CRDTypeAuthzRole
	if isClusterScoped(mapping.RoleRef.Namespace) {
		roleKind = CRDTypeAuthzClusterRole
	}

	// Update spec fields directly on the existing object
	existingBinding.Spec.Entitlement.Claim = mapping.Entitlement.Claim
	existingBinding.Spec.Entitlement.Value = mapping.Entitlement.Value
	existingBinding.Spec.RoleRef.Kind = roleKind
	existingBinding.Spec.RoleRef.Name = mapping.RoleRef.Name
	existingBinding.Spec.TargetPath.Project = mapping.Hierarchy.Project
	existingBinding.Spec.TargetPath.Component = mapping.Hierarchy.Component
	existingBinding.Spec.Effect = openchoreov1alpha1.EffectType(mapping.Effect)

	if err := ce.k8sClient.Update(ctx, existingBinding); err != nil {
		return fmt.Errorf("failed to update AuthzRoleBinding: %w", err)
	}

	ce.logger.Debug("updated AuthzRoleBinding", "name", mapping.Name, "namespace", mapping.Hierarchy.Namespace)
	return nil
}

// createClusterRole creates an AuthzClusterRole CRD
func (ce *CasbinEnforcer) createClusterRole(ctx context.Context, role *authzcore.Role) error {
	clusterRole := &openchoreov1alpha1.AuthzClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: role.Name,
		},
		Spec: openchoreov1alpha1.AuthzClusterRoleSpec{
			Actions:     role.Actions,
			Description: role.Description,
		},
	}

	if err := ce.k8sClient.Create(ctx, clusterRole); err != nil {
		return fmt.Errorf("failed to create AuthzClusterRole: %w", err)
	}

	ce.logger.Debug("created AuthzClusterRole", "name", role.Name)
	return nil
}

// createNamespacedRole creates an AuthzRole CRD in the specified namespace
func (ce *CasbinEnforcer) createNamespacedRole(ctx context.Context, role *authzcore.Role) error {
	namespacedRole := &openchoreov1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      role.Name,
			Namespace: role.Namespace,
		},
		Spec: openchoreov1alpha1.AuthzRoleSpec{
			Actions:     role.Actions,
			Description: role.Description,
		},
	}

	if err := ce.k8sClient.Create(ctx, namespacedRole); err != nil {
		return fmt.Errorf("failed to create AuthzRole: %w", err)
	}

	ce.logger.Debug("created AuthzRole", "name", role.Name, "namespace", role.Namespace)
	return nil
}

// deleteClusterRole deletes an AuthzClusterRole CRD
func (ce *CasbinEnforcer) deleteClusterRole(ctx context.Context, roleRef *authzcore.RoleRef) error {
	clusterRole := &openchoreov1alpha1.AuthzClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: roleRef.Name,
		},
	}

	if err := ce.k8sClient.Delete(ctx, clusterRole); err != nil {
		return fmt.Errorf("failed to delete AuthzClusterRole: %w", err)
	}

	ce.logger.Debug("deleted AuthzClusterRole", "name", roleRef.Name)
	return nil
}

// deleteNamespacedRole deletes an AuthzRole CRD from the specified namespace
func (ce *CasbinEnforcer) deleteNamespacedRole(ctx context.Context, roleRef *authzcore.RoleRef) error {
	namespacedRole := &openchoreov1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleRef.Name,
			Namespace: roleRef.Namespace,
		},
	}

	if err := ce.k8sClient.Delete(ctx, namespacedRole); err != nil {
		return fmt.Errorf("failed to delete AuthzRole: %w", err)
	}

	ce.logger.Debug("deleted AuthzRole", "name", roleRef.Name, "namespace", roleRef.Namespace)
	return nil
}

// deleteRoleBindingsForRole deletes all CRD bindings referencing a specific role
func (ce *CasbinEnforcer) deleteRoleBindingsForRole(ctx context.Context, roleRef *authzcore.RoleRef) error {
	namespace := normalizeNamespace(roleRef.Namespace)

	// Get all policies that reference this role using the Casbin enforcer
	mappingPolicies, err := ce.enforcer.GetFilteredPolicy(2, roleRef.Name, namespace)
	if err != nil {
		return fmt.Errorf("failed to get mappings using role: %w", err)
	}

	if len(mappingPolicies) == 0 {
		return nil
	}

	ce.logger.Debug("deleting role-entitlement binding CRDs",
		"role_name", roleRef.Name,
		"namespace", namespace,
		"mapping_count", len(mappingPolicies))

	// For each policy, we need to delete the corresponding CRD
	if isClusterScoped(roleRef.Namespace) {
		return ce.deleteBindingsForClusterRole(ctx, roleRef.Name)
	}
	return ce.deleteNamespacedBindingsForRole(ctx, roleRef.Name, roleRef.Namespace)
}

// deleteBindingsForClusterRole deletes all AuthzClusterRoleBindings referencing a cluster role
func (ce *CasbinEnforcer) deleteBindingsForClusterRole(ctx context.Context, roleName string) error {
	clusterBindingList := &openchoreov1alpha1.AuthzClusterRoleBindingList{}
	if err := ce.k8sClient.List(ctx, clusterBindingList); err != nil {
		return fmt.Errorf("failed to list AuthzClusterRoleBindings: %w", err)
	}

	for _, binding := range clusterBindingList.Items {
		if binding.Spec.RoleRef.Name == roleName && binding.Spec.RoleRef.Kind == CRDTypeAuthzClusterRole {
			if err := ce.k8sClient.Delete(ctx, &binding); err != nil {
				ce.logger.Error("failed to delete AuthzClusterRoleBinding", "name", binding.Name, "error", err)
			}
		}
	}

	// Also delete namespaced bindings that reference this cluster role
	roleBindingList := &openchoreov1alpha1.AuthzRoleBindingList{}
	if err := ce.k8sClient.List(ctx, roleBindingList); err != nil {
		return fmt.Errorf("failed to list AuthzRoleBindings: %w", err)
	}

	for _, binding := range roleBindingList.Items {
		if binding.Spec.RoleRef.Name == roleName && binding.Spec.RoleRef.Kind == CRDTypeAuthzClusterRole {
			if err := ce.k8sClient.Delete(ctx, &binding); err != nil {
				ce.logger.Error("failed to delete AuthzRoleBinding", "name", binding.Name, "namespace", binding.Namespace, "error", err)
			}
		}
	}

	return nil
}

// deleteNamespacedBindingsForRole deletes all AuthzRoleBindings in a namespace referencing a role
func (ce *CasbinEnforcer) deleteNamespacedBindingsForRole(ctx context.Context, roleName, namespace string) error {
	roleBindingList := &openchoreov1alpha1.AuthzRoleBindingList{}
	if err := ce.k8sClient.List(ctx, roleBindingList, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list AuthzRoleBindings in namespace %s: %w", namespace, err)
	}

	for _, binding := range roleBindingList.Items {
		if binding.Spec.RoleRef.Name == roleName && binding.Spec.RoleRef.Kind == "AuthzRole" {
			if err := ce.k8sClient.Delete(ctx, &binding); err != nil {
				ce.logger.Warn("failed to delete AuthzRoleBinding", "name", binding.Name, "namespace", binding.Namespace, "error", err)
			} else {
				ce.logger.Debug("deleted AuthzRoleBinding", "name", binding.Name, "namespace", binding.Namespace)
			}
		}
	}

	return nil
}

// updateClusterRoleActions updates an AuthzClusterRole's actions
func (ce *CasbinEnforcer) updateClusterRoleActions(ctx context.Context, role *authzcore.Role) error {
	clusterRole := &openchoreov1alpha1.AuthzClusterRole{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: role.Name}, clusterRole); err != nil {
		return fmt.Errorf("failed to get AuthzClusterRole: %w", err)
	}

	clusterRole.Spec.Actions = role.Actions

	if err := ce.k8sClient.Update(ctx, clusterRole); err != nil {
		return fmt.Errorf("failed to update AuthzClusterRole: %w", err)
	}

	ce.logger.Debug("updated AuthzClusterRole", "name", role.Name)
	return nil
}

// updateNamespacedRoleActions updates an AuthzRole's actions
func (ce *CasbinEnforcer) updateNamespacedRoleActions(ctx context.Context, role *authzcore.Role) error {
	namespacedRole := &openchoreov1alpha1.AuthzRole{}
	key := client.ObjectKey{Name: role.Name, Namespace: role.Namespace}
	if err := ce.k8sClient.Get(ctx, key, namespacedRole); err != nil {
		return fmt.Errorf("failed to get AuthzRole: %w", err)
	}

	namespacedRole.Spec.Actions = role.Actions

	if err := ce.k8sClient.Update(ctx, namespacedRole); err != nil {
		return fmt.Errorf("failed to update AuthzRole: %w", err)
	}

	ce.logger.Debug("updated AuthzRole", "name", role.Name, "namespace", role.Namespace)
	return nil
}

// buildBindingFromMapping converts core RoleEntitlementMapping to CRD binding objects
func (ce *CasbinEnforcer) buildBindingFromMapping(mapping *authzcore.RoleEntitlementMapping) client.Object {
	// Determine the role kind based on whether the role is cluster-scoped or not
	roleKind := CRDTypeAuthzRole
	if isClusterScoped(mapping.RoleRef.Namespace) {
		roleKind = CRDTypeAuthzClusterRole
	}

	// If hierarchy namespace is empty means cluster-scoped binding
	if isClusterScoped(mapping.Hierarchy.Namespace) {
		return &openchoreov1alpha1.AuthzClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: mapping.Name,
			},
			Spec: openchoreov1alpha1.AuthzClusterRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{
					Claim: mapping.Entitlement.Claim,
					Value: mapping.Entitlement.Value,
				},
				RoleRef: openchoreov1alpha1.RoleRef{
					Kind: roleKind,
					Name: mapping.RoleRef.Name,
				},
				Effect: openchoreov1alpha1.EffectType(mapping.Effect),
			},
		}
	}

	return &openchoreov1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mapping.Name,
			Namespace: mapping.Hierarchy.Namespace,
		},
		Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
			Entitlement: openchoreov1alpha1.EntitlementClaim{
				Claim: mapping.Entitlement.Claim,
				Value: mapping.Entitlement.Value,
			},
			RoleRef: openchoreov1alpha1.RoleRef{
				Kind: roleKind,
				Name: mapping.RoleRef.Name,
			},
			TargetPath: openchoreov1alpha1.TargetPath{
				Project:   mapping.Hierarchy.Project,
				Component: mapping.Hierarchy.Component,
			},
			Effect: openchoreov1alpha1.EffectType(mapping.Effect),
		},
	}
}

// convertBindingToMapping converts CRD bindings to core RoleEntitlementMapping objects
func (ce *CasbinEnforcer) convertBindingToMapping(binding interface{}) *authzcore.RoleEntitlementMapping {
	switch b := binding.(type) {
	case openchoreov1alpha1.AuthzRoleBinding:
		return &authzcore.RoleEntitlementMapping{
			Name: b.Name,
			RoleRef: authzcore.RoleRef{
				Name:      b.Spec.RoleRef.Name,
				Namespace: b.Namespace,
			},
			Entitlement: authzcore.Entitlement{
				Claim: b.Spec.Entitlement.Claim,
				Value: b.Spec.Entitlement.Value,
			},
			Hierarchy: authzcore.ResourceHierarchy{
				Namespace: b.Namespace,
				Project:   b.Spec.TargetPath.Project,
				Component: b.Spec.TargetPath.Component,
			},
			Effect: authzcore.PolicyEffectType(b.Spec.Effect),
		}
	case openchoreov1alpha1.AuthzClusterRoleBinding:
		return &authzcore.RoleEntitlementMapping{
			Name: b.Name,
			RoleRef: authzcore.RoleRef{
				Name:      b.Spec.RoleRef.Name,
				Namespace: "", // Cluster-scoped
			},
			Entitlement: authzcore.Entitlement{
				Claim: b.Spec.Entitlement.Claim,
				Value: b.Spec.Entitlement.Value,
			},
			Hierarchy: authzcore.ResourceHierarchy{}, // Cluster-wide, no hierarchy
			Effect:    authzcore.PolicyEffectType(b.Spec.Effect),
		}
	default:
		return nil
	}
}

// TODO: once context is properly integrated, pass it to enforcer
// check performs the actual authorization check using Casbin
func (ce *CasbinEnforcer) check(request *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	resourcePath := resourceHierarchyToPath(request.Resource.Hierarchy)
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
