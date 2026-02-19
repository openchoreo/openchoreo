// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

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
		filter = &authzcore.RoleFilter{IncludeAll: false}
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
		return ce.updateClusterRole(ctx, role)
	}
	return ce.updateNamespacedRole(ctx, role)
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
		if k8serrors.IsAlreadyExists(err) {
			return authzcore.ErrRoleMappingAlreadyExists
		}
		return fmt.Errorf("failed to create binding: %w", err)
	}

	ce.logger.Debug("created binding", "name", mapping.Name, "namespace", mapping.Hierarchy.Namespace)
	return nil
}

// GetRoleEntitlementMapping retrieves a role-entitlement mapping
func (ce *CasbinEnforcer) GetRoleEntitlementMapping(ctx context.Context, mappingRef *authzcore.MappingRef) (*authzcore.RoleEntitlementMapping, error) {
	if mappingRef == nil {
		return nil, fmt.Errorf("mappingRef cannot be nil")
	}

	ce.logger.Debug("get role entitlement mapping called",
		"mapping_name", mappingRef.Name,
		"mapping_namespace", mappingRef.Namespace)

	// can't utilize casbin engine as there's no reference about mapping name,
	// hence using k8s client to get the CRD directly
	if isClusterScoped(mappingRef.Namespace) {
		return ce.getClusterMapping(ctx, mappingRef)
	}
	return ce.getNamespacedMapping(ctx, mappingRef)
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
			if k8serrors.IsNotFound(err) {
				return authzcore.ErrRoleMappingNotFound
			}
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
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleMappingNotFound
		}
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

	// Policy format: [subject, resource, role, namespace, effect, context, binding_name]
	// Filter starting from index 0: subject, resource (skip with ""), role, namespace
	policies, err = ce.enforcer.GetFilteredPolicy(0, subject, "", roleName, roleNamespace, effect)
	if err != nil {
		return nil, fmt.Errorf("failed to get policies from enforcer: %w", err)
	}

	mappings := make([]*authzcore.RoleEntitlementMapping, 0, len(policies))

	for _, policy := range policies {
		if len(policy) != 7 {
			ce.logger.Warn("skipping malformed policy", "policy", policy, "expected", 7, "got", len(policy))
			continue
		}

		// Policy format: [subject, resource, role, namespace, effect, context, binding_name]
		policySubject := policy[0]
		resourcePath := policy[1]
		policyRole := policy[2]
		policyNamespace := policy[3]
		policyEffect := policy[4]
		// policy[5] is context
		bindingName := policy[6]

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
			Name: bindingName,
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

// CreateClusterRole creates a new cluster-scoped role and returns the full CRD object
func (ce *CasbinEnforcer) CreateClusterRole(ctx context.Context, role *openchoreov1alpha1.AuthzClusterRole) (*openchoreov1alpha1.AuthzClusterRole, error) {
	if role == nil {
		return nil, fmt.Errorf("%w: cluster role cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("create cluster role called", "name", role.Name)

	if err := ce.k8sClient.Create(ctx, role); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil, authzcore.ErrRoleAlreadyExists
		}
		return nil, fmt.Errorf("failed to create AuthzClusterRole: %w", err)
	}
	return role, nil
}

// GetClusterRole retrieves a cluster-scoped role by name
func (ce *CasbinEnforcer) GetClusterRole(ctx context.Context, name string) (*openchoreov1alpha1.AuthzClusterRole, error) {
	role := &openchoreov1alpha1.AuthzClusterRole{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: name}, role); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to get AuthzClusterRole: %w", err)
	}
	return role, nil
}

// ListClusterRoles lists all cluster-scoped roles
func (ce *CasbinEnforcer) ListClusterRoles(ctx context.Context) (*openchoreov1alpha1.AuthzClusterRoleList, error) {
	list := &openchoreov1alpha1.AuthzClusterRoleList{}
	if err := ce.k8sClient.List(ctx, list); err != nil {
		return nil, fmt.Errorf("failed to list AuthzClusterRoles: %w", err)
	}
	return list, nil
}

// UpdateClusterRole updates a cluster-scoped role and returns the updated CRD object
func (ce *CasbinEnforcer) UpdateClusterRole(ctx context.Context, role *openchoreov1alpha1.AuthzClusterRole) (*openchoreov1alpha1.AuthzClusterRole, error) {
	if role == nil {
		return nil, fmt.Errorf("%w: cluster role cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("update cluster role called", "name", role.Name)

	existing := &openchoreov1alpha1.AuthzClusterRole{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: role.Name}, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to get AuthzClusterRole: %w", err)
	}

	// Apply incoming spec directly, preserving server-managed ObjectMeta fields
	existing.Labels = role.Labels
	existing.Annotations = role.Annotations
	existing.Spec = role.Spec

	if err := ce.k8sClient.Update(ctx, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to update AuthzClusterRole: %w", err)
	}
	return existing, nil
}

// CreateNamespacedRole creates a new namespace-scoped role and returns the full CRD object
func (ce *CasbinEnforcer) CreateNamespacedRole(ctx context.Context, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	if role == nil {
		return nil, fmt.Errorf("%w: namespaced role cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("create namespaced role called", "name", role.Name, "namespace", role.Namespace)

	if err := ce.k8sClient.Create(ctx, role); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil, authzcore.ErrRoleAlreadyExists
		}
		return nil, fmt.Errorf("failed to create AuthzRole: %w", err)
	}
	return role, nil
}

// GetNamespacedRole retrieves a namespace-scoped role by name and namespace
func (ce *CasbinEnforcer) GetNamespacedRole(ctx context.Context, name string, namespace string) (*openchoreov1alpha1.AuthzRole, error) {
	role := &openchoreov1alpha1.AuthzRole{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, role); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to get AuthzRole: %w", err)
	}
	return role, nil
}

// ListNamespacedRoles lists namespace-scoped roles in the given namespace
func (ce *CasbinEnforcer) ListNamespacedRoles(ctx context.Context, namespace string) (*openchoreov1alpha1.AuthzRoleList, error) {
	list := &openchoreov1alpha1.AuthzRoleList{}
	if err := ce.k8sClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list AuthzRoles: %w", err)
	}
	return list, nil
}

// UpdateNamespacedRole updates a namespace-scoped role and returns the updated CRD object
func (ce *CasbinEnforcer) UpdateNamespacedRole(ctx context.Context, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	if role == nil {
		return nil, fmt.Errorf("%w: namespaced role cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("update namespaced role called", "name", role.Name, "namespace", role.Namespace)

	existing := &openchoreov1alpha1.AuthzRole{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: role.Name, Namespace: role.Namespace}, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to get AuthzRole: %w", err)
	}

	// Apply incoming spec directly, preserving server-managed ObjectMeta fields
	existing.Labels = role.Labels
	existing.Annotations = role.Annotations
	existing.Spec = role.Spec

	if err := ce.k8sClient.Update(ctx, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to update AuthzRole: %w", err)
	}
	return existing, nil
}

// CreateClusterRoleBinding creates a new cluster-scoped role binding and returns the full CRD object
func (ce *CasbinEnforcer) CreateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzClusterRoleBinding) (*openchoreov1alpha1.AuthzClusterRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("%w: cluster role binding cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("create cluster role binding called", "name", binding.Name)

	if err := ce.k8sClient.Create(ctx, binding); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil, authzcore.ErrRoleMappingAlreadyExists
		}
		return nil, fmt.Errorf("failed to create AuthzClusterRoleBinding: %w", err)
	}
	return binding, nil
}

// GetClusterRoleBinding retrieves a cluster-scoped role binding by name
func (ce *CasbinEnforcer) GetClusterRoleBinding(ctx context.Context, name string) (*openchoreov1alpha1.AuthzClusterRoleBinding, error) {
	binding := &openchoreov1alpha1.AuthzClusterRoleBinding{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: name}, binding); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleMappingNotFound
		}
		return nil, fmt.Errorf("failed to get AuthzClusterRoleBinding: %w", err)
	}
	return binding, nil
}

// ListClusterRoleBindings lists all cluster-scoped role bindings
func (ce *CasbinEnforcer) ListClusterRoleBindings(ctx context.Context) (*openchoreov1alpha1.AuthzClusterRoleBindingList, error) {
	list := &openchoreov1alpha1.AuthzClusterRoleBindingList{}
	if err := ce.k8sClient.List(ctx, list); err != nil {
		return nil, fmt.Errorf("failed to list AuthzClusterRoleBindings: %w", err)
	}
	return list, nil
}

// UpdateClusterRoleBinding updates a cluster-scoped role binding and returns the updated CRD object
func (ce *CasbinEnforcer) UpdateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzClusterRoleBinding) (*openchoreov1alpha1.AuthzClusterRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("%w: cluster role binding cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("update cluster role binding called", "name", binding.Name)

	existing := &openchoreov1alpha1.AuthzClusterRoleBinding{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: binding.Name}, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleMappingNotFound
		}
		return nil, fmt.Errorf("failed to get AuthzClusterRoleBinding: %w", err)
	}

	// Apply incoming spec directly, preserving server-managed ObjectMeta fields
	existing.Labels = binding.Labels
	existing.Annotations = binding.Annotations
	existing.Spec = binding.Spec

	if err := ce.k8sClient.Update(ctx, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleMappingNotFound
		}
		return nil, fmt.Errorf("failed to update AuthzClusterRoleBinding: %w", err)
	}
	return existing, nil
}

// CreateNamespacedRoleBinding creates a new namespace-scoped role binding and returns the full CRD object
func (ce *CasbinEnforcer) CreateNamespacedRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("%w: namespaced role binding cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("create namespaced role binding called", "name", binding.Name, "namespace", binding.Namespace)

	if err := ce.k8sClient.Create(ctx, binding); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil, authzcore.ErrRoleMappingAlreadyExists
		}
		return nil, fmt.Errorf("failed to create AuthzRoleBinding: %w", err)
	}
	return binding, nil
}

// GetNamespacedRoleBinding retrieves a namespace-scoped role binding by name and namespace
func (ce *CasbinEnforcer) GetNamespacedRoleBinding(ctx context.Context, name string, namespace string) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	binding := &openchoreov1alpha1.AuthzRoleBinding{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, binding); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleMappingNotFound
		}
		return nil, fmt.Errorf("failed to get AuthzRoleBinding: %w", err)
	}
	return binding, nil
}

// ListNamespacedRoleBindings lists namespace-scoped role bindings in the given namespace
func (ce *CasbinEnforcer) ListNamespacedRoleBindings(ctx context.Context, namespace string) (*openchoreov1alpha1.AuthzRoleBindingList, error) {
	list := &openchoreov1alpha1.AuthzRoleBindingList{}
	if err := ce.k8sClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list AuthzRoleBindings: %w", err)
	}
	return list, nil
}

// UpdateNamespacedRoleBinding updates a namespace-scoped role binding and returns the updated CRD object
func (ce *CasbinEnforcer) UpdateNamespacedRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("%w: namespaced role binding cannot be nil", authzcore.ErrInvalidRequest)
	}
	ce.logger.Debug("update namespaced role binding called", "name", binding.Name, "namespace", binding.Namespace)

	existing := &openchoreov1alpha1.AuthzRoleBinding{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: binding.Name, Namespace: binding.Namespace}, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleMappingNotFound
		}
		return nil, fmt.Errorf("failed to get AuthzRoleBinding: %w", err)
	}

	// Apply incoming spec directly, preserving server-managed ObjectMeta fields
	existing.Labels = binding.Labels
	existing.Annotations = binding.Annotations
	existing.Spec = binding.Spec

	if err := ce.k8sClient.Update(ctx, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleMappingNotFound
		}
		return nil, fmt.Errorf("failed to update AuthzRoleBinding: %w", err)
	}
	return existing, nil
}

// ============================================================================
// K8s CRD Helper Methods
// ============================================================================

// getClusterMapping retrieves a cluster-scoped role-entitlement mapping
func (ce *CasbinEnforcer) getClusterMapping(ctx context.Context, mappingRef *authzcore.MappingRef) (*authzcore.RoleEntitlementMapping, error) {
	clusterBinding := &openchoreov1alpha1.AuthzClusterRoleBinding{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: mappingRef.Name}, clusterBinding); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleMappingNotFound
		}
		return nil, fmt.Errorf("failed to get AuthzClusterRoleBinding: %w", err)
	}

	ce.logger.Debug("retrieved AuthzClusterRoleBinding", "name", mappingRef.Name)
	return ce.convertBindingToMapping(*clusterBinding), nil
}

// getNamespacedMapping retrieves a namespaced role-entitlement mapping
func (ce *CasbinEnforcer) getNamespacedMapping(ctx context.Context, mappingRef *authzcore.MappingRef) (*authzcore.RoleEntitlementMapping, error) {
	roleBinding := &openchoreov1alpha1.AuthzRoleBinding{}
	key := client.ObjectKey{Name: mappingRef.Name, Namespace: mappingRef.Namespace}
	if err := ce.k8sClient.Get(ctx, key, roleBinding); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, authzcore.ErrRoleMappingNotFound
		}
		return nil, fmt.Errorf("failed to get AuthzRoleBinding: %w", err)
	}

	ce.logger.Debug("retrieved AuthzRoleBinding", "name", mappingRef.Name, "namespace", mappingRef.Namespace)
	return ce.convertBindingToMapping(*roleBinding), nil
}

// updateClusterRoleBinding updates an existing AuthzClusterRoleBinding
func (ce *CasbinEnforcer) updateClusterRoleBinding(ctx context.Context, mapping *authzcore.RoleEntitlementMapping) error {
	existingBinding := &openchoreov1alpha1.AuthzClusterRoleBinding{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: mapping.Name}, existingBinding); err != nil {
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleMappingNotFound
		}
		return fmt.Errorf("cluster role binding not found: %w", err)
	}

	// Determine the role kind based on whether the role is cluster-scoped or not
	roleKind := openchoreov1alpha1.RoleRefKindAuthzRole
	if isClusterScoped(mapping.RoleRef.Namespace) {
		roleKind = openchoreov1alpha1.RoleRefKindAuthzClusterRole
	}

	// Update spec fields directly on the existing object
	existingBinding.Spec.Entitlement.Claim = mapping.Entitlement.Claim
	existingBinding.Spec.Entitlement.Value = mapping.Entitlement.Value
	existingBinding.Spec.RoleRef.Kind = roleKind
	existingBinding.Spec.RoleRef.Name = mapping.RoleRef.Name
	existingBinding.Spec.Effect = openchoreov1alpha1.EffectType(mapping.Effect)

	if err := ce.k8sClient.Update(ctx, existingBinding); err != nil {
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleMappingNotFound
		}
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
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleMappingNotFound
		}
		return fmt.Errorf("role binding not found: %w", err)
	}

	// Determine the role kind based on whether the role is cluster-scoped or not
	roleKind := openchoreov1alpha1.RoleRefKindAuthzRole
	if isClusterScoped(mapping.RoleRef.Namespace) {
		roleKind = openchoreov1alpha1.RoleRefKindAuthzClusterRole
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
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleMappingNotFound
		}
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
		if k8serrors.IsAlreadyExists(err) {
			return authzcore.ErrRoleAlreadyExists
		}
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
		if k8serrors.IsAlreadyExists(err) {
			return authzcore.ErrRoleAlreadyExists
		}
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
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleNotFound
		}
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
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleNotFound
		}
		return fmt.Errorf("failed to delete AuthzRole: %w", err)
	}

	ce.logger.Debug("deleted AuthzRole", "name", roleRef.Name, "namespace", roleRef.Namespace)
	return nil
}

// updateClusterRole updates an AuthzClusterRole's actions
func (ce *CasbinEnforcer) updateClusterRole(ctx context.Context, role *authzcore.Role) error {
	clusterRole := &openchoreov1alpha1.AuthzClusterRole{}
	if err := ce.k8sClient.Get(ctx, client.ObjectKey{Name: role.Name}, clusterRole); err != nil {
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleNotFound
		}
		return fmt.Errorf("failed to get AuthzClusterRole: %w", err)
	}

	clusterRole.Spec.Actions = role.Actions
	clusterRole.Spec.Description = role.Description

	if err := ce.k8sClient.Update(ctx, clusterRole); err != nil {
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleNotFound
		}
		return fmt.Errorf("failed to update AuthzClusterRole: %w", err)
	}

	ce.logger.Debug("updated AuthzClusterRole", "name", role.Name)
	return nil
}

// updateNamespacedRole updates an AuthzRole's actions
func (ce *CasbinEnforcer) updateNamespacedRole(ctx context.Context, role *authzcore.Role) error {
	namespacedRole := &openchoreov1alpha1.AuthzRole{}
	key := client.ObjectKey{Name: role.Name, Namespace: role.Namespace}
	if err := ce.k8sClient.Get(ctx, key, namespacedRole); err != nil {
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleNotFound
		}
		return fmt.Errorf("failed to get AuthzRole: %w", err)
	}

	namespacedRole.Spec.Actions = role.Actions
	namespacedRole.Spec.Description = role.Description

	if err := ce.k8sClient.Update(ctx, namespacedRole); err != nil {
		if k8serrors.IsNotFound(err) {
			return authzcore.ErrRoleNotFound
		}
		return fmt.Errorf("failed to update AuthzRole: %w", err)
	}

	ce.logger.Debug("updated AuthzRole", "name", role.Name, "namespace", role.Namespace)
	return nil
}

// buildBindingFromMapping converts core RoleEntitlementMapping to CRD binding objects
func (ce *CasbinEnforcer) buildBindingFromMapping(mapping *authzcore.RoleEntitlementMapping) client.Object {
	// Determine the role kind based on whether the role is cluster-scoped or not
	roleKind := openchoreov1alpha1.RoleRefKindAuthzRole
	if isClusterScoped(mapping.RoleRef.Namespace) {
		roleKind = openchoreov1alpha1.RoleRefKindAuthzClusterRole
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
		ns := b.Namespace
		if b.Spec.RoleRef.Kind == CRDTypeAuthzClusterRole {
			ns = ""
		}
		return &authzcore.RoleEntitlementMapping{
			Name: b.Name,
			RoleRef: authzcore.RoleRef{
				Name:      b.Spec.RoleRef.Name,
				Namespace: ns,
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
				Namespace: "",
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
