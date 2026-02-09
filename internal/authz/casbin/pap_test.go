// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	"errors"
	"slices"
	"testing"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

const (
	testClaimGroups = "groups"
)

// ============================================================================
// PAP Tests - Role Management
// ============================================================================

// TestCasbinEnforcer_AddRole_ClusterRole tests adding cluster-scoped roles
func TestCasbinEnforcer_AddRole_ClusterRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("add cluster-scoped role", func(t *testing.T) {
		role := &authzcore.Role{
			Name:    testRoleName,
			Actions: []string{"component:view", "component:create"},
		}
		err := enforcer.AddRole(ctx, role)
		if err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		var crd openchoreov1alpha1.AuthzClusterRole
		err = enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: testRoleName}, &crd)
		if err != nil {
			t.Fatalf("AuthzClusterRole CRD not created: %v", err)
		}

		if !sliceContainsSameElements(crd.Spec.Actions, role.Actions) {
			t.Errorf("CRD actions = %v, want %v", crd.Spec.Actions, role.Actions)
		}
	})

	t.Run("add duplicate role", func(t *testing.T) {
		role := &authzcore.Role{
			Name:    "duplicate-role",
			Actions: []string{"component:view"},
		}

		// Add role first time
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() first call error = %v", err)
		}

		// Add role second time - should fail with ErrRoleAlreadyExists
		err := enforcer.AddRole(ctx, role)
		if !errors.Is(err, authzcore.ErrRoleAlreadyExists) {
			t.Errorf("AddRole() second call error = %v, want ErrRoleAlreadyExists", err)
		}
	})
}

// TestCasbinEnforcer_AddRole_NamespacedRole tests adding namespace-scoped roles
func TestCasbinEnforcer_AddRole_NamespacedRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	t.Run("add namespace-scoped role", func(t *testing.T) {
		role := &authzcore.Role{
			Name:      "ns-developer",
			Namespace: testNs,
			Actions:   []string{"component:create", "component:view"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("failed to add namespace role: %v", err)
		}

		// Verify: AuthzRole CRD was created in K8s
		var crd openchoreov1alpha1.AuthzRole
		err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "ns-developer", Namespace: testNs}, &crd)
		if err != nil {
			t.Fatalf("AuthzRole CRD not created: %v", err)
		}

		// Verify CRD spec matches
		if !sliceContainsSameElements(crd.Spec.Actions, role.Actions) {
			t.Errorf("CRD actions = %v, want %v", crd.Spec.Actions, role.Actions)
		}
	})

	t.Run("cluster and namespace roles with same name", func(t *testing.T) {
		// Create cluster role
		clusterRole := &authzcore.Role{
			Name:    "admin",
			Actions: []string{"*"},
		}
		if err := enforcer.AddRole(ctx, clusterRole); err != nil {
			t.Fatalf("failed to add cluster role: %v", err)
		}

		// Create namespace role with same name
		nsRole := &authzcore.Role{
			Name:      "admin",
			Namespace: testNs,
			Actions:   []string{"component:view"},
		}
		if err := enforcer.AddRole(ctx, nsRole); err != nil {
			t.Fatalf("failed to add namespace role: %v", err)
		}

		// Verify: Both CRDs exist in K8s
		var clusterCrd openchoreov1alpha1.AuthzClusterRole
		err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "admin"}, &clusterCrd)
		if err != nil {
			t.Fatalf("AuthzClusterRole CRD not created: %v", err)
		}

		var nsCrd openchoreov1alpha1.AuthzRole
		err = enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "admin", Namespace: testNs}, &nsCrd)
		if err != nil {
			t.Fatalf("AuthzRole CRD not created: %v", err)
		}

		// Verify specs match
		if !sliceContainsSameElements(clusterCrd.Spec.Actions, clusterRole.Actions) {
			t.Errorf("ClusterRole CRD actions = %v, want %v", clusterCrd.Spec.Actions, clusterRole.Actions)
		}
		if !sliceContainsSameElements(nsCrd.Spec.Actions, nsRole.Actions) {
			t.Errorf("Role CRD actions = %v, want %v", nsCrd.Spec.Actions, nsRole.Actions)
		}
	})

	t.Run("multiple namespace roles with same name in different namespaces", func(t *testing.T) {
		role1 := &authzcore.Role{
			Name:      "engineer",
			Namespace: testNs,
			Actions:   []string{"component:create"},
		}
		if err := enforcer.AddRole(ctx, role1); err != nil {
			t.Fatalf("failed to add first namespace role: %v", err)
		}

		role2 := &authzcore.Role{
			Name:      "engineer",
			Namespace: "widgets",
			Actions:   []string{"component:update"},
		}
		if err := enforcer.AddRole(ctx, role2); err != nil {
			t.Fatalf("failed to add second namespace role: %v", err)
		}

		// Verify: Both AuthzRole CRDs exist in their respective namespaces
		var crd1 openchoreov1alpha1.AuthzRole
		err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "engineer", Namespace: testNs}, &crd1)
		if err != nil {
			t.Fatalf("AuthzRole CRD in %s not created: %v", testNs, err)
		}

		var crd2 openchoreov1alpha1.AuthzRole
		err = enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "engineer", Namespace: "widgets"}, &crd2)
		if err != nil {
			t.Fatalf("AuthzRole CRD in widgets not created: %v", err)
		}

		// Verify specs match
		if !sliceContainsSameElements(crd1.Spec.Actions, role1.Actions) {
			t.Errorf("Role1 CRD actions = %v, want %v", crd1.Spec.Actions, role1.Actions)
		}
		if !sliceContainsSameElements(crd2.Spec.Actions, role2.Actions) {
			t.Errorf("Role2 CRD actions = %v, want %v", crd2.Spec.Actions, role2.Actions)
		}
	})
}

// TestCasbinEnforcer_GetRole tests retrieving cluster-scoped roles
func TestCasbinEnforcer_GetRole_ClusterRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	_, err := enforcer.enforcer.AddGroupingPolicy("test-admin", "*", "*")
	if err != nil {
		t.Fatalf("AddGroupingPolicy() error = %v", err)
	}

	// Test getting the created role
	role, err := enforcer.GetRole(ctx, &authzcore.RoleRef{Name: "test-admin"})
	if err != nil {
		t.Fatalf("GetRole() error = %v", err)
	}

	if role.Name != "test-admin" {
		t.Errorf("GetRole() name = %s, want test-admin", role.Name)
	}

	if !slices.Contains(role.Actions, "*") {
		t.Error("GetRole() test-admin should have wildcard action")
	}
}

// TestCasbinEnforcer_GetRole_NamespacedRole tests retrieving namespace-scoped roles
func TestCasbinEnforcer_GetRole_NamespacedRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	// Setup: Create namespace role directly using Casbin grouping policies
	// Format: g, <role>, <action>, <namespace>
	// Using "acme" for namespace-scoped role
	roleRules := [][]string{
		{"ns-viewer", "component:view", testNs},
		{"ns-viewer", "project:view", testNs},
	}
	_, err := enforcer.enforcer.AddGroupingPolicies(roleRules)
	if err != nil {
		t.Fatalf("failed to add namespace role via grouping policies: %v", err)
	}

	t.Run("get existing namespace role", func(t *testing.T) {
		fetched, err := enforcer.GetRole(ctx, &authzcore.RoleRef{Name: "ns-viewer", Namespace: testNs})
		if err != nil {
			t.Fatalf("failed to get namespace role: %v", err)
		}
		if fetched.Name != "ns-viewer" {
			t.Errorf("expected name 'ns-viewer', got '%s'", fetched.Name)
		}
		if fetched.Namespace != testNs {
			t.Errorf("expected namespace '%s', got '%s'", testNs, fetched.Namespace)
		}
		if len(fetched.Actions) != 2 {
			t.Errorf("expected 2 actions, got %d", len(fetched.Actions))
		}
	})

	t.Run("get non-existent namespace role", func(t *testing.T) {
		_, err := enforcer.GetRole(ctx, &authzcore.RoleRef{Name: "non-existent", Namespace: testNs})
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("expected ErrRoleNotFound, got %v", err)
		}
	})

	t.Run("get namespace role with wrong namespace", func(t *testing.T) {
		_, err := enforcer.GetRole(ctx, &authzcore.RoleRef{Name: "ns-viewer", Namespace: "wrong-ns"})
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("expected ErrRoleNotFound, got %v", err)
		}
	})
}

// TestCasbinEnforcer_ListRoles tests listing roles
func TestCasbinEnforcer_ListRoles(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	// Setup: Create multiple roles (cluster and namespace scoped)
	roleRules := [][]string{
		{"cluster-admin", "*", "*"},
		{"ns-dev-1", "component:create", testNs},
		{"ns-dev-2", "component:view", testNs},
		{"ns-viewer", "component:view", "widgets"},
	}
	_, err := enforcer.enforcer.AddGroupingPolicies(roleRules)
	if err != nil {
		t.Fatalf("failed to add roles via grouping policies: %v", err)
	}

	t.Run("list roles in specific namespace", func(t *testing.T) {
		filter := authzcore.RoleFilter{Namespace: testNs}
		fetched, err := enforcer.ListRoles(ctx, &filter)
		if err != nil {
			t.Fatalf("failed to list roles: %v", err)
		}
		if len(fetched) != 2 {
			t.Errorf("expected 2 roles in '%s' namespace, got %d", testNs, len(fetched))
		}
		for _, role := range fetched {
			if role.Namespace != testNs {
				t.Errorf("expected namespace '%s', got '%s'", testNs, role.Namespace)
			}
		}
	})

	t.Run("list all roles including cluster and namespace", func(t *testing.T) {
		filter := authzcore.RoleFilter{IncludeAll: true}
		fetched, err := enforcer.ListRoles(ctx, &filter)
		if err != nil {
			t.Fatalf("failed to list roles: %v", err)
		}
		if len(fetched) < 4 {
			t.Errorf("expected at least 4 roles, got %d", len(fetched))
		}
	})

	t.Run("list only cluster roles", func(t *testing.T) {
		filter := authzcore.RoleFilter{}
		fetched, err := enforcer.ListRoles(ctx, &filter)
		if err != nil {
			t.Fatalf("failed to list roles: %v", err)
		}
		if len(fetched) < 1 {
			t.Errorf("expected at least 1 cluster role, got %d", len(fetched))
		}
		for _, role := range fetched {
			if role.Namespace != "" {
				t.Errorf("expected cluster role with empty namespace, got '%s'", role.Namespace)
			}
		}
	})
}

// TestCasbinEnforcer_RemoveRole_ClusterRole tests removing cluster-scoped roles
func TestCasbinEnforcer_RemoveRole_ClusterRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("remove existing role", func(t *testing.T) {
		// Setup: Create role
		role := &authzcore.Role{
			Name:    "removable-role",
			Actions: []string{"component:view"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		var crd openchoreov1alpha1.AuthzClusterRole
		if err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "removable-role"}, &crd); err != nil {
			t.Fatalf("AuthzClusterRole CRD not created: %v", err)
		}

		if err := enforcer.RemoveRole(ctx, &authzcore.RoleRef{Name: "removable-role"}); err != nil {
			t.Fatalf("RemoveRole() error = %v", err)
		}

		err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "removable-role"}, &crd)
		if !k8serrors.IsNotFound(err) {
			t.Errorf("RemoveRole() AuthzClusterRole CRD still exists, expected NotFound error, got: %v", err)
		}
	})

	t.Run("non-existent role", func(t *testing.T) {
		err := enforcer.RemoveRole(ctx, &authzcore.RoleRef{Name: "non-existent-role"})
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("RemoveRole() error = %v, want ErrRoleNotFound", err)
		}
	})
}

// TestCasbinEnforcer_RemoveRole_NamespacedRole tests removing namespace-scoped roles
func TestCasbinEnforcer_RemoveRole_NamespacedRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	t.Run("remove existing namespace role", func(t *testing.T) {
		role := &authzcore.Role{
			Name:      "unused-role",
			Namespace: testNs,
			Actions:   []string{"component:view"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Verify CRD exists before removal
		var crd openchoreov1alpha1.AuthzRole
		if err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "unused-role", Namespace: testNs}, &crd); err != nil {
			t.Fatalf("AuthzRole CRD not created: %v", err)
		}

		if err := enforcer.RemoveRole(ctx, &authzcore.RoleRef{Name: "unused-role", Namespace: testNs}); err != nil {
			t.Fatalf("RemoveRole() error = %v", err)
		}

		err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "unused-role", Namespace: testNs}, &crd)
		if !k8serrors.IsNotFound(err) {
			t.Errorf("RemoveRole() AuthzRole CRD still exists, expected NotFound error, got: %v", err)
		}
	})
}

// TestCasbinEnforcer_UpdateRole_ClusterRole tests updating cluster-scoped roles
// Verifies that AuthzClusterRole CRD spec is updated in K8s
func TestCasbinEnforcer_UpdateRole_ClusterRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("update role with both added and removed actions", func(t *testing.T) {
		// Setup: Create role
		role := &authzcore.Role{
			Name:    "mixed-update-role",
			Actions: []string{"component:view", "component:create", "project:view"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Call PAP method to update the role
		updatedRole := &authzcore.Role{
			Name:    "mixed-update-role",
			Actions: []string{"component:view", "component:delete"},
		}
		if err := enforcer.UpdateRole(ctx, updatedRole); err != nil {
			t.Fatalf("UpdateRole() error = %v", err)
		}

		// Verify: AuthzClusterRole CRD spec was updated in K8s
		var crd openchoreov1alpha1.AuthzClusterRole
		if err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "mixed-update-role"}, &crd); err != nil {
			t.Fatalf("Failed to get AuthzClusterRole CRD: %v", err)
		}

		// Verify CRD spec has updated actions
		if len(crd.Spec.Actions) != 2 {
			t.Errorf("UpdateRole() CRD has %d actions, want 2", len(crd.Spec.Actions))
		}

		if !slices.Contains(crd.Spec.Actions, "component:view") {
			t.Error("UpdateRole() CRD missing component:view action")
		}
		if !slices.Contains(crd.Spec.Actions, "component:delete") {
			t.Error("UpdateRole() CRD missing component:delete action")
		}
		if slices.Contains(crd.Spec.Actions, "component:create") {
			t.Error("UpdateRole() CRD should not have component:create action")
		}
		if slices.Contains(crd.Spec.Actions, "project:view") {
			t.Error("UpdateRole() CRD should not have project:view action")
		}
	})

	t.Run("update role with empty actions should fail", func(t *testing.T) {
		// Setup: Create role
		role := &authzcore.Role{
			Name:    "empty-actions-role",
			Actions: []string{"component:view", "component:create"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Try to update with empty actions - should fail
		updatedRole := &authzcore.Role{
			Name:    "empty-actions-role",
			Actions: []string{},
		}
		err := enforcer.UpdateRole(ctx, updatedRole)
		if err == nil {
			t.Error("UpdateRole() with empty actions should return error")
		}

		// Verify: AuthzClusterRole CRD spec is unchanged in K8s
		var crd openchoreov1alpha1.AuthzClusterRole
		if err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "empty-actions-role"}, &crd); err != nil {
			t.Fatalf("Failed to get AuthzClusterRole CRD: %v", err)
		}

		if len(crd.Spec.Actions) != 2 {
			t.Errorf("UpdateRole() failed but CRD actions changed, got %d actions, want 2", len(crd.Spec.Actions))
		}
	})

	t.Run("update non-existent role", func(t *testing.T) {
		nonExistentRole := &authzcore.Role{
			Name:    "does-not-exist",
			Actions: []string{"component:view"},
		}
		err := enforcer.UpdateRole(ctx, nonExistentRole)
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("UpdateRole() error = %v, want ErrRoleNotFound", err)
		}
	})
}

// TestCasbinEnforcer_UpdateRole_NamespacedRole tests updating namespace-scoped roles
// Verifies that AuthzRole CRD spec is updated in K8s
func TestCasbinEnforcer_UpdateRole_NamespacedRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	t.Run("update namespace role actions", func(t *testing.T) {
		// Setup: Create role
		role := &authzcore.Role{
			Name:      "ns-engineer",
			Namespace: testNs,
			Actions:   []string{"component:create", "component:view"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Call PAP method to update the role
		updatedRole := &authzcore.Role{
			Name:      "ns-engineer",
			Namespace: testNs,
			Actions:   []string{"component:create", "component:view", "component:update"},
		}
		if err := enforcer.UpdateRole(ctx, updatedRole); err != nil {
			t.Fatalf("UpdateRole() error = %v", err)
		}

		// Verify: AuthzRole CRD spec was updated in K8s
		var crd openchoreov1alpha1.AuthzRole
		if err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "ns-engineer", Namespace: testNs}, &crd); err != nil {
			t.Fatalf("Failed to get AuthzRole CRD: %v", err)
		}

		// Verify CRD spec has updated actions
		if len(crd.Spec.Actions) != 3 {
			t.Errorf("UpdateRole() CRD has %d actions, want 3", len(crd.Spec.Actions))
		}

		expectedActions := []string{"component:create", "component:view", "component:update"}
		for _, action := range expectedActions {
			if !slices.Contains(crd.Spec.Actions, action) {
				t.Errorf("UpdateRole() CRD missing expected action: %s", action)
			}
		}
	})

	t.Run("update non-existent namespace role", func(t *testing.T) {
		nonExistent := &authzcore.Role{
			Name:      "non-existent",
			Namespace: testNs,
			Actions:   []string{"component:view"},
		}
		err := enforcer.UpdateRole(ctx, nonExistent)
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("expected ErrRoleNotFound, got %v", err)
		}
	})
}

// ============================================================================
// PAP Tests - Role Entitlement Mapping Management
// ============================================================================

// TestCasbinEnforcer_AddRoleEntitlementMapping tests adding role-entitlement mappings
// Verifies that AuthzRoleBinding/AuthzClusterRoleBinding CRD is created in K8s
func TestCasbinEnforcer_AddRoleEntitlementMapping(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	t.Run("add namespaced role binding", func(t *testing.T) {
		// Setup: Create role
		role := &authzcore.Role{
			Name:    "binding-test-role",
			Actions: []string{"component:view"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Call PAP method to add the mapping
		mapping := &authzcore.RoleEntitlementMapping{
			Name: "test-binding",
			Entitlement: authzcore.Entitlement{
				Claim: testClaimGroups,
				Value: "test-group",
			},
			RoleRef: authzcore.RoleRef{Name: "binding-test-role"},
			Hierarchy: authzcore.ResourceHierarchy{
				Namespace: testNs,
			},
			Effect: authzcore.PolicyEffectAllow,
		}

		if err := enforcer.AddRoleEntitlementMapping(ctx, mapping); err != nil {
			t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
		}

		// Verify: AuthzRoleBinding CRD was created in K8s
		var crd openchoreov1alpha1.AuthzRoleBinding
		if err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "test-binding", Namespace: testNs}, &crd); err != nil {
			t.Fatalf("AuthzRoleBinding CRD not created: %v", err)
		}

		// Verify CRD spec matches
		if crd.Spec.Entitlement.Claim != testClaimGroups {
			t.Errorf("CRD entitlement claim = %s, want groups", crd.Spec.Entitlement.Claim)
		}
		if crd.Spec.Entitlement.Value != "test-group" {
			t.Errorf("CRD entitlement value = %s, want test-group", crd.Spec.Entitlement.Value)
		}
		if crd.Spec.RoleRef.Name != "binding-test-role" {
			t.Errorf("CRD roleRef name = %s, want binding-test-role", crd.Spec.RoleRef.Name)
		}
		if crd.Spec.Effect != openchoreov1alpha1.EffectAllow {
			t.Errorf("CRD effect = %s, want allow", crd.Spec.Effect)
		}
	})

	t.Run("add cluster role binding", func(t *testing.T) {
		// Setup: Create cluster role
		role := &authzcore.Role{
			Name:    "cluster-binding-test-role",
			Actions: []string{"component:view"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Call PAP method to add the cluster-scoped mapping
		mapping := &authzcore.RoleEntitlementMapping{
			Name: "test-cluster-binding",
			Entitlement: authzcore.Entitlement{
				Claim: testClaimGroups,
				Value: "admin-group",
			},
			RoleRef: authzcore.RoleRef{Name: "cluster-binding-test-role"},
			// Empty namespace means cluster-scoped binding
			Hierarchy: authzcore.ResourceHierarchy{},
			Effect:    authzcore.PolicyEffectAllow,
		}

		if err := enforcer.AddRoleEntitlementMapping(ctx, mapping); err != nil {
			t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
		}

		// Verify: AuthzClusterRoleBinding CRD was created in K8s
		var crd openchoreov1alpha1.AuthzClusterRoleBinding
		if err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "test-cluster-binding"}, &crd); err != nil {
			t.Fatalf("AuthzClusterRoleBinding CRD not created: %v", err)
		}

		// Verify CRD spec matches
		if crd.Spec.Entitlement.Claim != testClaimGroups {
			t.Errorf("CRD entitlement claim = %s, want groups", crd.Spec.Entitlement.Claim)
		}
		if crd.Spec.Entitlement.Value != "admin-group" {
			t.Errorf("CRD entitlement value = %s, want admin-group", crd.Spec.Entitlement.Value)
		}
		if crd.Spec.RoleRef.Name != "cluster-binding-test-role" {
			t.Errorf("CRD roleRef name = %s, want cluster-binding-test-role", crd.Spec.RoleRef.Name)
		}
	})

	t.Run("add binding with project scope", func(t *testing.T) {
		// Setup: Create role
		role := &authzcore.Role{
			Name:    "project-binding-role",
			Actions: []string{"component:deploy"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Call PAP method to add the mapping with project scope
		mapping := &authzcore.RoleEntitlementMapping{
			Name: "project-scoped-binding",
			Entitlement: authzcore.Entitlement{
				Claim: testClaimGroups,
				Value: "dev-group",
			},
			RoleRef: authzcore.RoleRef{Name: "project-binding-role"},
			Hierarchy: authzcore.ResourceHierarchy{
				Namespace: testNs,
				Project:   "project1",
			},
			Effect: authzcore.PolicyEffectAllow,
		}

		if err := enforcer.AddRoleEntitlementMapping(ctx, mapping); err != nil {
			t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
		}

		// Verify: AuthzRoleBinding CRD was created in K8s
		var crd openchoreov1alpha1.AuthzRoleBinding
		if err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "project-scoped-binding", Namespace: testNs}, &crd); err != nil {
			t.Fatalf("AuthzRoleBinding CRD not created: %v", err)
		}

		// Verify CRD spec has targetPath with project
		if crd.Spec.TargetPath.Project != "project1" {
			t.Errorf("CRD targetPath.project = %s, want project1", crd.Spec.TargetPath.Project)
		}
	})

	t.Run("add binding with deny effect", func(t *testing.T) {
		// Setup: Create role
		role := &authzcore.Role{
			Name:    "deny-binding-role",
			Actions: []string{"component:delete"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Call PAP method to add the mapping with deny effect
		mapping := &authzcore.RoleEntitlementMapping{
			Name: "deny-binding",
			Entitlement: authzcore.Entitlement{
				Claim: testClaimGroups,
				Value: "restricted-group",
			},
			RoleRef: authzcore.RoleRef{Name: "deny-binding-role"},
			Hierarchy: authzcore.ResourceHierarchy{
				Namespace: testNs,
			},
			Effect: authzcore.PolicyEffectDeny,
		}

		if err := enforcer.AddRoleEntitlementMapping(ctx, mapping); err != nil {
			t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
		}

		// Verify: AuthzRoleBinding CRD was created with deny effect
		var crd openchoreov1alpha1.AuthzRoleBinding
		if err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "deny-binding", Namespace: testNs}, &crd); err != nil {
			t.Fatalf("AuthzRoleBinding CRD not created: %v", err)
		}

		if crd.Spec.Effect != openchoreov1alpha1.EffectDeny {
			t.Errorf("CRD effect = %s, want deny", crd.Spec.Effect)
		}
	})
}

// TestCasbinEnforcer_RemoveRoleEntitlementMapping tests removing role-entitlement mappings
// Verifies that AuthzRoleBinding/AuthzClusterRoleBinding CRD is deleted from K8s
func TestCasbinEnforcer_RemoveRoleEntitlementMapping(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	t.Run("remove namespaced role binding", func(t *testing.T) {
		// Setup: Create role
		role := &authzcore.Role{
			Name:    "remove-binding-role",
			Actions: []string{"component:view"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Create binding
		bindingName := "remove-test-binding"
		mapping := &authzcore.RoleEntitlementMapping{
			Name: bindingName,
			Entitlement: authzcore.Entitlement{
				Claim: testEntitlementType,
				Value: testEntitlementValue,
			},
			RoleRef: authzcore.RoleRef{Name: "remove-binding-role"},
			Hierarchy: authzcore.ResourceHierarchy{
				Namespace: testNs,
			},
			Effect: authzcore.PolicyEffectAllow,
		}

		if err := enforcer.AddRoleEntitlementMapping(ctx, mapping); err != nil {
			t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
		}

		// Verify CRD exists before removal
		var crd openchoreov1alpha1.AuthzRoleBinding
		if err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: testNs}, &crd); err != nil {
			t.Fatalf("AuthzRoleBinding CRD not created: %v", err)
		}

		// Call PAP method to remove the binding
		mappingRef := &authzcore.MappingRef{
			Name:      bindingName,
			Namespace: testNs,
		}
		if err := enforcer.RemoveRoleEntitlementMapping(ctx, mappingRef); err != nil {
			t.Fatalf("RemoveRoleEntitlementMapping() error = %v", err)
		}

		// Verify: AuthzRoleBinding CRD was deleted from K8s
		err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: testNs}, &crd)
		if !k8serrors.IsNotFound(err) {
			t.Errorf("RemoveRoleEntitlementMapping() AuthzRoleBinding CRD still exists, expected NotFound error, got: %v", err)
		}
	})

	t.Run("remove cluster role binding", func(t *testing.T) {
		// Setup: Create cluster role
		role := &authzcore.Role{
			Name:    "remove-cluster-binding-role",
			Actions: []string{"component:view"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Create cluster binding
		bindingName := "remove-cluster-binding"
		mapping := &authzcore.RoleEntitlementMapping{
			Name: bindingName,
			Entitlement: authzcore.Entitlement{
				Claim: testClaimGroups,
				Value: "admin-group",
			},
			RoleRef:   authzcore.RoleRef{Name: "remove-cluster-binding-role"},
			Hierarchy: authzcore.ResourceHierarchy{}, // Empty namespace = cluster-scoped
			Effect:    authzcore.PolicyEffectAllow,
		}

		if err := enforcer.AddRoleEntitlementMapping(ctx, mapping); err != nil {
			t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
		}

		// Verify CRD exists before removal
		var crd openchoreov1alpha1.AuthzClusterRoleBinding
		if err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: bindingName}, &crd); err != nil {
			t.Fatalf("AuthzClusterRoleBinding CRD not created: %v", err)
		}

		// Call PAP method to remove the binding
		mappingRef := &authzcore.MappingRef{
			Name:      bindingName,
			Namespace: "", // Empty namespace = cluster-scoped
		}
		if err := enforcer.RemoveRoleEntitlementMapping(ctx, mappingRef); err != nil {
			t.Fatalf("RemoveRoleEntitlementMapping() error = %v", err)
		}

		// Verify: AuthzClusterRoleBinding CRD was deleted from K8s
		err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: bindingName}, &crd)
		if !k8serrors.IsNotFound(err) {
			t.Errorf("RemoveRoleEntitlementMapping() AuthzClusterRoleBinding CRD still exists, expected NotFound error, got: %v", err)
		}
	})

	t.Run("remove non-existent binding", func(t *testing.T) {
		mappingRef := &authzcore.MappingRef{
			Name:      "non-existent-binding",
			Namespace: testNs,
		}
		err := enforcer.RemoveRoleEntitlementMapping(ctx, mappingRef)
		if !errors.Is(err, authzcore.ErrRoleMappingNotFound) {
			t.Errorf("RemoveRoleEntitlementMapping() error = %v, want ErrRoleMappingNotFound", err)
		}
	})
}

// TestCasbinEnforcer_UpdateRoleEntitlementMapping tests updating role-entitlement mappings
// Verifies that AuthzRoleBinding/AuthzClusterRoleBinding CRD spec is updated in K8s
func TestCasbinEnforcer_UpdateRoleEntitlementMapping(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	t.Run("update existing namespaced mapping", func(t *testing.T) {
		// Setup: Create role
		role := &authzcore.Role{
			Name:    "update-mapping-role",
			Actions: []string{"component:view"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Create binding
		bindingName := "update-test-binding"
		mapping := &authzcore.RoleEntitlementMapping{
			Name: bindingName,
			Entitlement: authzcore.Entitlement{
				Claim: testClaimGroups,
				Value: "dev-group",
			},
			RoleRef: authzcore.RoleRef{Name: "update-mapping-role"},
			Hierarchy: authzcore.ResourceHierarchy{
				Namespace: testNs,
			},
			Effect: authzcore.PolicyEffectAllow,
		}
		if err := enforcer.AddRoleEntitlementMapping(ctx, mapping); err != nil {
			t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
		}

		// Verify CRD was created
		var crd openchoreov1alpha1.AuthzRoleBinding
		if err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: testNs}, &crd); err != nil {
			t.Fatalf("AuthzRoleBinding CRD not created: %v", err)
		}

		// Call PAP method to update the binding
		updatedMapping := &authzcore.RoleEntitlementMapping{
			Name: bindingName,
			Entitlement: authzcore.Entitlement{
				Claim: testClaimGroups,
				Value: "prod-group",
			},
			RoleRef: authzcore.RoleRef{Name: "update-mapping-role"},
			Hierarchy: authzcore.ResourceHierarchy{
				Namespace: testNs,
				Project:   "p1",
			},
			Effect: authzcore.PolicyEffectDeny,
		}

		if err := enforcer.UpdateRoleEntitlementMapping(ctx, updatedMapping); err != nil {
			t.Fatalf("UpdateRoleEntitlementMapping() error = %v", err)
		}

		// Verify: AuthzRoleBinding CRD spec was updated in K8s
		if err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: testNs}, &crd); err != nil {
			t.Fatalf("Failed to get AuthzRoleBinding CRD: %v", err)
		}

		// Verify CRD spec has updated values
		if crd.Spec.Entitlement.Value != "prod-group" {
			t.Errorf("UpdateRoleEntitlementMapping() CRD entitlement value = %s, want prod-group", crd.Spec.Entitlement.Value)
		}
		if crd.Spec.TargetPath.Project != "p1" {
			t.Errorf("UpdateRoleEntitlementMapping() CRD targetPath.project = %s, want p1", crd.Spec.TargetPath.Project)
		}
		if crd.Spec.Effect != openchoreov1alpha1.EffectDeny {
			t.Errorf("UpdateRoleEntitlementMapping() CRD effect = %s, want deny", crd.Spec.Effect)
		}
	})

	t.Run("update non-existent mapping", func(t *testing.T) {
		// Setup: Create role
		role := &authzcore.Role{
			Name:    "nonexistent-mapping-role",
			Actions: []string{"component:view"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		mapping := &authzcore.RoleEntitlementMapping{
			Name: "non-existent-mapping",
			Entitlement: authzcore.Entitlement{
				Claim: testClaimGroups,
				Value: "test",
			},
			RoleRef: authzcore.RoleRef{Name: "nonexistent-mapping-role"},
			Hierarchy: authzcore.ResourceHierarchy{
				Namespace: testNs,
			},
			Effect: authzcore.PolicyEffectAllow,
		}

		err := enforcer.UpdateRoleEntitlementMapping(ctx, mapping)
		if !errors.Is(err, authzcore.ErrRoleMappingNotFound) {
			t.Errorf("UpdateRoleEntitlementMapping() error = %v, want ErrRoleMappingNotFound", err)
		}
	})
}

// TestCasbinEnforcer_ListActions tests listing all actions
func TestCasbinEnforcer_ListActions(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	actions, err := enforcer.ListActions(ctx)
	if err != nil {
		t.Fatalf("ListActions() error = %v", err)
	}

	if len(actions) == 0 {
		t.Error("ListActions() returned empty list")
	}
}
