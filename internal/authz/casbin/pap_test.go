// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

const (
	testClaimGroups = "groups"
)

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

// TestCasbinEnforcer_CreateClusterRole tests creating cluster-scoped roles
func TestCasbinEnforcer_CreateClusterRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("create cluster role", func(t *testing.T) {
		role := &openchoreov1alpha1.ClusterAuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "crd-admin"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleSpec{
				Actions:     []string{"component:view", "component:create"},
				Description: "admin role",
			},
		}
		created, err := enforcer.CreateClusterRole(ctx, role)
		if err != nil {
			t.Fatalf("CreateClusterRole() error = %v", err)
		}
		if created.Name != "crd-admin" {
			t.Errorf("CreateClusterRole() name = %s, want crd-admin", created.Name)
		}
		if !sliceContainsSameElements(created.Spec.Actions, []string{"component:view", "component:create"}) {
			t.Errorf("CreateClusterRole() actions = %v, want [component:view component:create]", created.Spec.Actions)
		}
	})

	t.Run("create duplicate cluster role", func(t *testing.T) {
		role := &openchoreov1alpha1.ClusterAuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "dup-cluster-role"},
			Spec:       openchoreov1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:view"}},
		}
		if _, err := enforcer.CreateClusterRole(ctx, role); err != nil {
			t.Fatalf("CreateClusterRole() first call error = %v", err)
		}
		_, err := enforcer.CreateClusterRole(ctx, &openchoreov1alpha1.ClusterAuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "dup-cluster-role"},
			Spec:       openchoreov1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:view"}},
		})
		if !errors.Is(err, authzcore.ErrRoleAlreadyExists) {
			t.Errorf("CreateClusterRole() duplicate error = %v, want ErrRoleAlreadyExists", err)
		}
	})

	t.Run("invalid cluster role", func(t *testing.T) {
		_, err := enforcer.CreateClusterRole(ctx, nil)
		if !errors.Is(err, authzcore.ErrInvalidRequest) {
			t.Errorf("CreateClusterRole(nil) error = %v, want ErrInvalidRequest", err)
		}
	})
}

// TestCasbinEnforcer_GetClusterRole tests fetching cluster-scoped roles
func TestCasbinEnforcer_GetClusterRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// Setup: create a cluster role via k8s client
	role := &openchoreov1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "get-cluster-role"},
		Spec:       openchoreov1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:view"}},
	}
	if err := enforcer.k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("setup: failed to create cluster role: %v", err)
	}

	t.Run("get existing cluster role", func(t *testing.T) {
		fetched, err := enforcer.GetClusterRole(ctx, "get-cluster-role")
		if err != nil {
			t.Fatalf("GetClusterRole() error = %v", err)
		}
		if fetched.Name != "get-cluster-role" {
			t.Errorf("GetClusterRole() name = %s, want get-cluster-role", fetched.Name)
		}
		if !sliceContainsSameElements(fetched.Spec.Actions, []string{"component:view"}) {
			t.Errorf("GetClusterRole() actions = %v, want [component:view]", fetched.Spec.Actions)
		}
	})

	t.Run("get non-existent cluster role", func(t *testing.T) {
		_, err := enforcer.GetClusterRole(ctx, "non-existent")
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("GetClusterRole() error = %v, want ErrRoleNotFound", err)
		}
	})
}

// TestCasbinEnforcer_ListClusterRoles tests listing cluster-scoped roles
func TestCasbinEnforcer_ListClusterRoles(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// Setup: create two cluster roles
	for _, name := range []string{"list-cr-1", "list-cr-2"} {
		role := &openchoreov1alpha1.ClusterAuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       openchoreov1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:view"}},
		}
		if err := enforcer.k8sClient.Create(ctx, role); err != nil {
			t.Fatalf("setup: failed to create cluster role %s: %v", name, err)
		}
	}

	list, err := enforcer.ListClusterRoles(ctx, 0, "")
	if err != nil {
		t.Fatalf("ListClusterRoles() error = %v", err)
	}
	if len(list.Items) < 2 {
		t.Errorf("ListClusterRoles() returned %d items, want at least 2", len(list.Items))
	}
}

// TestCasbinEnforcer_UpdateClusterRole tests updating cluster-scoped roles
func TestCasbinEnforcer_UpdateClusterRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("update existing cluster role", func(t *testing.T) {
		role := &openchoreov1alpha1.ClusterAuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "update-cr"},
			Spec:       openchoreov1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:view"}},
		}
		if err := enforcer.k8sClient.Create(ctx, role); err != nil {
			t.Fatalf("setup: %v", err)
		}

		updated, err := enforcer.UpdateClusterRole(ctx, &openchoreov1alpha1.ClusterAuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "update-cr"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleSpec{
				Actions:     []string{"component:view", "component:delete"},
				Description: "updated",
			},
		})
		if err != nil {
			t.Fatalf("UpdateClusterRole() error = %v", err)
		}
		if !sliceContainsSameElements(updated.Spec.Actions, []string{"component:view", "component:delete"}) {
			t.Errorf("UpdateClusterRole() actions = %v", updated.Spec.Actions)
		}
		if updated.Spec.Description != "updated" {
			t.Errorf("UpdateClusterRole() description = %s, want updated", updated.Spec.Description)
		}
	})

	t.Run("update non-existent cluster role", func(t *testing.T) {
		_, err := enforcer.UpdateClusterRole(ctx, &openchoreov1alpha1.ClusterAuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "does-not-exist"},
			Spec:       openchoreov1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:view"}},
		})
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("UpdateClusterRole() error = %v, want ErrRoleNotFound", err)
		}
	})

	t.Run("update invalid cluster role", func(t *testing.T) {
		_, err := enforcer.UpdateClusterRole(ctx, nil)
		if !errors.Is(err, authzcore.ErrInvalidRequest) {
			t.Errorf("UpdateClusterRole(nil) error = %v, want ErrInvalidRequest", err)
		}
	})
}

// TestCasbinEnforcer_DeleteClusterRole tests deleting cluster-scoped roles
func TestCasbinEnforcer_DeleteClusterRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("delete existing cluster role", func(t *testing.T) {
		role := &openchoreov1alpha1.ClusterAuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "delete-cr"},
			Spec:       openchoreov1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:view"}},
		}
		if err := enforcer.k8sClient.Create(ctx, role); err != nil {
			t.Fatalf("setup: %v", err)
		}

		if err := enforcer.DeleteClusterRole(ctx, "delete-cr"); err != nil {
			t.Fatalf("DeleteClusterRole() error = %v", err)
		}

		// Verify the CRD is gone
		var crd openchoreov1alpha1.ClusterAuthzRole
		err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "delete-cr"}, &crd)
		if !k8serrors.IsNotFound(err) {
			t.Errorf("expected NotFound after delete, got err = %v", err)
		}
	})

	t.Run("delete non-existent cluster role", func(t *testing.T) {
		err := enforcer.DeleteClusterRole(ctx, "non-existent-cr")
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("DeleteClusterRole() error = %v, want ErrRoleNotFound", err)
		}
	})
}

// TestCasbinEnforcer_CreateNamespacedRole tests creating namespace-scoped roles
func TestCasbinEnforcer_CreateNamespacedRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	t.Run("create namespaced role", func(t *testing.T) {
		role := &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "ns-dev", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleSpec{
				Actions: []string{"component:view", "component:create"},
			},
		}
		created, err := enforcer.CreateNamespacedRole(ctx, role)
		if err != nil {
			t.Fatalf("CreateNamespacedRole() error = %v", err)
		}
		if created.Name != "ns-dev" || created.Namespace != testNs {
			t.Errorf("CreateNamespacedRole() = %s/%s, want %s/ns-dev", created.Namespace, created.Name, testNs)
		}
	})

	t.Run("create duplicate namespaced role", func(t *testing.T) {
		role := &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "dup-ns-role", Namespace: testNs},
			Spec:       openchoreov1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
		}
		if _, err := enforcer.CreateNamespacedRole(ctx, role); err != nil {
			t.Fatalf("first call error = %v", err)
		}
		_, err := enforcer.CreateNamespacedRole(ctx, &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "dup-ns-role", Namespace: testNs},
			Spec:       openchoreov1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
		})
		if !errors.Is(err, authzcore.ErrRoleAlreadyExists) {
			t.Errorf("duplicate error = %v, want ErrRoleAlreadyExists", err)
		}
	})

	t.Run("create nil namespaced role", func(t *testing.T) {
		_, err := enforcer.CreateNamespacedRole(ctx, nil)
		if !errors.Is(err, authzcore.ErrInvalidRequest) {
			t.Errorf("CreateNamespacedRole(nil) error = %v, want ErrInvalidRequest", err)
		}
	})
}

// TestCasbinEnforcer_GetNamespacedRole tests fetching namespace-scoped roles
func TestCasbinEnforcer_GetNamespacedRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	role := &openchoreov1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "get-ns-role", Namespace: testNs},
		Spec:       openchoreov1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
	}
	if err := enforcer.k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Run("get existing namespaced role", func(t *testing.T) {
		fetched, err := enforcer.GetNamespacedRole(ctx, "get-ns-role", testNs)
		if err != nil {
			t.Fatalf("GetNamespacedRole() error = %v", err)
		}
		if fetched.Name != "get-ns-role" || fetched.Namespace != testNs {
			t.Errorf("GetNamespacedRole() = %s/%s", fetched.Namespace, fetched.Name)
		}
	})

	t.Run("get non-existent namespaced role", func(t *testing.T) {
		_, err := enforcer.GetNamespacedRole(ctx, "non-existent", testNs)
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("error = %v, want ErrRoleNotFound", err)
		}
	})

	t.Run("get namespaced role wrong namespace", func(t *testing.T) {
		_, err := enforcer.GetNamespacedRole(ctx, "get-ns-role", "wrong-ns")
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("error = %v, want ErrRoleNotFound", err)
		}
	})
}

// TestCasbinEnforcer_ListNamespacedRoles tests listing namespace-scoped roles
func TestCasbinEnforcer_ListNamespacedRoles(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	for _, r := range []struct {
		name string
		ns   string
	}{
		{"list-ns-1", testNs},
		{"list-ns-2", testNs},
		{"list-ns-other", "other-ns"},
	} {
		role := &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: r.name, Namespace: r.ns},
			Spec:       openchoreov1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
		}
		if err := enforcer.k8sClient.Create(ctx, role); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	t.Run("list roles in specific namespace", func(t *testing.T) {
		list, err := enforcer.ListNamespacedRoles(ctx, testNs, 0, "")
		if err != nil {
			t.Fatalf("ListNamespacedRoles() error = %v", err)
		}
		if len(list.Items) != 2 {
			t.Errorf("ListNamespacedRoles() returned %d items, want 2", len(list.Items))
		}
		for _, item := range list.Items {
			if item.Namespace != testNs {
				t.Errorf("item namespace = %s, want %s", item.Namespace, testNs)
			}
		}
	})

	t.Run("list roles in empty namespace", func(t *testing.T) {
		list, err := enforcer.ListNamespacedRoles(ctx, "empty-ns", 0, "")
		if err != nil {
			t.Fatalf("ListNamespacedRoles() error = %v", err)
		}
		if len(list.Items) != 0 {
			t.Errorf("ListNamespacedRoles() returned %d items, want 0", len(list.Items))
		}
	})
}

// TestCasbinEnforcer_UpdateNamespacedRole tests updating namespace-scoped roles
func TestCasbinEnforcer_UpdateNamespacedRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	t.Run("update existing namespaced role", func(t *testing.T) {
		role := &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "update-ns-role", Namespace: testNs},
			Spec:       openchoreov1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
		}
		if err := enforcer.k8sClient.Create(ctx, role); err != nil {
			t.Fatalf("setup: %v", err)
		}

		updated, err := enforcer.UpdateNamespacedRole(ctx, &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "update-ns-role", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleSpec{
				Actions:     []string{"component:view", "component:delete"},
				Description: "updated desc",
			},
		})
		if err != nil {
			t.Fatalf("UpdateNamespacedRole() error = %v", err)
		}
		if !sliceContainsSameElements(updated.Spec.Actions, []string{"component:view", "component:delete"}) {
			t.Errorf("UpdateNamespacedRole() actions = %v", updated.Spec.Actions)
		}
	})

	t.Run("update non-existent namespaced role", func(t *testing.T) {
		_, err := enforcer.UpdateNamespacedRole(ctx, &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "no-exist", Namespace: testNs},
			Spec:       openchoreov1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
		})
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("error = %v, want ErrRoleNotFound", err)
		}
	})

	t.Run("update nil namespaced role", func(t *testing.T) {
		_, err := enforcer.UpdateNamespacedRole(ctx, nil)
		if !errors.Is(err, authzcore.ErrInvalidRequest) {
			t.Errorf("error = %v, want ErrInvalidRequest", err)
		}
	})
}

// TestCasbinEnforcer_DeleteNamespacedRole tests deleting namespace-scoped roles
func TestCasbinEnforcer_DeleteNamespacedRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	t.Run("delete existing namespaced role", func(t *testing.T) {
		role := &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "delete-ns-role", Namespace: testNs},
			Spec:       openchoreov1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
		}
		if err := enforcer.k8sClient.Create(ctx, role); err != nil {
			t.Fatalf("setup: %v", err)
		}

		if err := enforcer.DeleteNamespacedRole(ctx, "delete-ns-role", testNs); err != nil {
			t.Fatalf("DeleteNamespacedRole() error = %v", err)
		}

		// Verify the CRD is gone
		var crd openchoreov1alpha1.AuthzRole
		err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "delete-ns-role", Namespace: testNs}, &crd)
		if !k8serrors.IsNotFound(err) {
			t.Errorf("expected NotFound after delete, got err = %v", err)
		}
	})

	t.Run("delete non-existent namespaced role", func(t *testing.T) {
		err := enforcer.DeleteNamespacedRole(ctx, "non-existent", testNs)
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("DeleteNamespacedRole() error = %v, want ErrRoleNotFound", err)
		}
	})

	t.Run("delete namespaced role wrong namespace", func(t *testing.T) {
		role := &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "delete-wrong-ns", Namespace: testNs},
			Spec:       openchoreov1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
		}
		if err := enforcer.k8sClient.Create(ctx, role); err != nil {
			t.Fatalf("setup: %v", err)
		}

		err := enforcer.DeleteNamespacedRole(ctx, "delete-wrong-ns", "wrong-ns")
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("DeleteNamespacedRole() error = %v, want ErrRoleNotFound", err)
		}
	})
}

// TestCasbinEnforcer_CreateClusterRoleBinding tests creating cluster-scoped role bindings
func TestCasbinEnforcer_CreateClusterRoleBinding(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("create cluster role binding with single mapping", func(t *testing.T) {
		wantMappings := []openchoreov1alpha1.ClusterRoleMapping{{
			RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"},
		}}
		binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "crd-crb-1"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "admins"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectAllow,
			},
		}
		created, err := enforcer.CreateClusterRoleBinding(ctx, binding)
		if err != nil {
			t.Fatalf("CreateClusterRoleBinding() error = %v", err)
		}
		if created.Name != "crd-crb-1" {
			t.Errorf("name = %s, want crd-crb-1", created.Name)
		}
		if created.Spec.Entitlement.Value != "admins" {
			t.Errorf("entitlement value = %s, want admins", created.Spec.Entitlement.Value)
		}
		if len(created.Spec.RoleMappings) != 1 {
			t.Fatalf("RoleMappings length = %d, want 1", len(created.Spec.RoleMappings))
		}
		if !reflect.DeepEqual(created.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", created.Spec.RoleMappings, wantMappings)
		}
		if created.Spec.Effect != openchoreov1alpha1.EffectAllow {
			t.Errorf("effect = %s, want allow", created.Spec.Effect)
		}
	})

	t.Run("create duplicate cluster role binding", func(t *testing.T) {
		binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "dup-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "devs"},
				RoleMappings: []openchoreov1alpha1.ClusterRoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "dev"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		}
		if _, err := enforcer.CreateClusterRoleBinding(ctx, binding); err != nil {
			t.Fatalf("first call error = %v", err)
		}
		_, err := enforcer.CreateClusterRoleBinding(ctx, &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "dup-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "devs"},
				RoleMappings: []openchoreov1alpha1.ClusterRoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "dev"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		})
		if !errors.Is(err, authzcore.ErrRoleMappingAlreadyExists) {
			t.Errorf("duplicate error = %v, want ErrRoleMappingAlreadyExists", err)
		}
	})

	t.Run("create cluster role binding with multiple role mappings", func(t *testing.T) {
		wantMappings := []openchoreov1alpha1.ClusterRoleMapping{
			{RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"}},
			{RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "viewer"}},
			{RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "editor"}},
		}
		binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "multi-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "multi-group"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectAllow,
			},
		}
		created, err := enforcer.CreateClusterRoleBinding(ctx, binding)
		if err != nil {
			t.Fatalf("CreateClusterRoleBinding() error = %v", err)
		}
		if len(created.Spec.RoleMappings) != len(wantMappings) {
			t.Fatalf("RoleMappings length = %d, want %d", len(created.Spec.RoleMappings), len(wantMappings))
		}
		if !reflect.DeepEqual(created.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", created.Spec.RoleMappings, wantMappings)
		}
		if created.Spec.Effect != openchoreov1alpha1.EffectAllow {
			t.Errorf("effect = %s, want allow", created.Spec.Effect)
		}

		// Verify round-trip via Get
		fetched, err := enforcer.GetClusterRoleBinding(ctx, "multi-crb")
		if err != nil {
			t.Fatalf("GetClusterRoleBinding() error = %v", err)
		}
		if len(fetched.Spec.RoleMappings) != len(wantMappings) {
			t.Fatalf("fetched RoleMappings length = %d, want %d", len(fetched.Spec.RoleMappings), len(wantMappings))
		}
		if !reflect.DeepEqual(fetched.Spec.RoleMappings, wantMappings) {
			t.Errorf("fetched RoleMappings = %v, want %v", fetched.Spec.RoleMappings, wantMappings)
		}
		if fetched.Spec.Effect != openchoreov1alpha1.EffectAllow {
			t.Errorf("fetched effect = %s, want allow", fetched.Spec.Effect)
		}
	})

	t.Run("create cluster role binding with scoped mapping", func(t *testing.T) {
		wantMappings := []openchoreov1alpha1.ClusterRoleMapping{{
			RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "ns-editor"},
			Scope:   openchoreov1alpha1.ClusterTargetScope{Namespace: "acme", Project: "p1"},
		}}
		binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "scoped-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "scoped-group"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectAllow,
			},
		}
		created, err := enforcer.CreateClusterRoleBinding(ctx, binding)
		if err != nil {
			t.Fatalf("CreateClusterRoleBinding() error = %v", err)
		}
		if len(created.Spec.RoleMappings) != 1 {
			t.Fatalf("RoleMappings length = %d, want 1", len(created.Spec.RoleMappings))
		}
		if !reflect.DeepEqual(created.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", created.Spec.RoleMappings, wantMappings)
		}

		// Verify round-trip via Get
		fetched, err := enforcer.GetClusterRoleBinding(ctx, "scoped-crb")
		if err != nil {
			t.Fatalf("GetClusterRoleBinding() error = %v", err)
		}
		if !reflect.DeepEqual(fetched.Spec.RoleMappings, wantMappings) {
			t.Errorf("fetched RoleMappings = %v, want %v", fetched.Spec.RoleMappings, wantMappings)
		}
	})

	t.Run("create invalid cluster role binding", func(t *testing.T) {
		_, err := enforcer.CreateClusterRoleBinding(ctx, nil)
		if !errors.Is(err, authzcore.ErrInvalidRequest) {
			t.Errorf("error = %v, want ErrInvalidRequest", err)
		}
	})
}

// TestCasbinEnforcer_GetClusterRoleBinding tests fetching cluster-scoped role bindings
func TestCasbinEnforcer_GetClusterRoleBinding(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("get existing with single mapping", func(t *testing.T) {
		wantMappings := []openchoreov1alpha1.ClusterRoleMapping{{
			RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "ops-role"},
		}}
		binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "get-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "ops"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectAllow,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}

		fetched, err := enforcer.GetClusterRoleBinding(ctx, "get-crb")
		if err != nil {
			t.Fatalf("GetClusterRoleBinding() error = %v", err)
		}
		if len(fetched.Spec.RoleMappings) != 1 {
			t.Fatalf("RoleMappings length = %d, want 1", len(fetched.Spec.RoleMappings))
		}
		if !reflect.DeepEqual(fetched.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", fetched.Spec.RoleMappings, wantMappings)
		}
		if fetched.Spec.Effect != openchoreov1alpha1.EffectAllow {
			t.Errorf("effect = %s, want allow", fetched.Spec.Effect)
		}
	})

	t.Run("get existing with multiple mappings", func(t *testing.T) {
		wantMappings := []openchoreov1alpha1.ClusterRoleMapping{
			{RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "role-x"}},
			{RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "role-y"}},
		}
		binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "get-multi-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "multi-ops"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectDeny,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}

		fetched, err := enforcer.GetClusterRoleBinding(ctx, "get-multi-crb")
		if err != nil {
			t.Fatalf("GetClusterRoleBinding() error = %v", err)
		}
		if len(fetched.Spec.RoleMappings) != len(wantMappings) {
			t.Fatalf("RoleMappings length = %d, want %d", len(fetched.Spec.RoleMappings), len(wantMappings))
		}
		if !reflect.DeepEqual(fetched.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", fetched.Spec.RoleMappings, wantMappings)
		}
		if fetched.Spec.Effect != openchoreov1alpha1.EffectDeny {
			t.Errorf("effect = %s, want deny", fetched.Spec.Effect)
		}
	})

	t.Run("get existing with scoped mapping", func(t *testing.T) {
		wantMappings := []openchoreov1alpha1.ClusterRoleMapping{{
			RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "ns-role"},
			Scope:   openchoreov1alpha1.ClusterTargetScope{Namespace: "acme", Project: "p1"},
		}}
		binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "get-scoped-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "scoped-ops"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectAllow,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}

		fetched, err := enforcer.GetClusterRoleBinding(ctx, "get-scoped-crb")
		if err != nil {
			t.Fatalf("GetClusterRoleBinding() error = %v", err)
		}
		if !reflect.DeepEqual(fetched.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", fetched.Spec.RoleMappings, wantMappings)
		}
	})

	t.Run("get non-existent", func(t *testing.T) {
		_, err := enforcer.GetClusterRoleBinding(ctx, "non-existent")
		if !errors.Is(err, authzcore.ErrRoleMappingNotFound) {
			t.Errorf("error = %v, want ErrRoleMappingNotFound", err)
		}
	})
}

// TestCasbinEnforcer_ListClusterRoleBindings tests listing cluster-scoped role bindings
func TestCasbinEnforcer_ListClusterRoleBindings(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	for _, name := range []string{"list-crb-1", "list-crb-2"} {
		binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "g"},
				RoleMappings: []openchoreov1alpha1.ClusterRoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "r"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	list, err := enforcer.ListClusterRoleBindings(ctx, 0, "")
	if err != nil {
		t.Fatalf("ListClusterRoleBindings() error = %v", err)
	}
	if len(list.Items) < 2 {
		t.Errorf("ListClusterRoleBindings() returned %d items, want at least 2", len(list.Items))
	}
}

// TestCasbinEnforcer_UpdateClusterRoleBinding tests updating cluster-scoped role bindings
func TestCasbinEnforcer_UpdateClusterRoleBinding(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("update existing cluster role binding with single mapping", func(t *testing.T) {
		binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "update-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "old-group"},
				RoleMappings: []openchoreov1alpha1.ClusterRoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "old-role"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}

		wantMappings := []openchoreov1alpha1.ClusterRoleMapping{{
			RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "new-role"},
		}}
		updated, err := enforcer.UpdateClusterRoleBinding(ctx, &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "update-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "new-group"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectDeny,
			},
		})
		if err != nil {
			t.Fatalf("UpdateClusterRoleBinding() error = %v", err)
		}
		if updated.Spec.Entitlement.Value != "new-group" {
			t.Errorf("entitlement value = %s, want new-group", updated.Spec.Entitlement.Value)
		}
		if len(updated.Spec.RoleMappings) != 1 {
			t.Fatalf("RoleMappings length = %d, want 1", len(updated.Spec.RoleMappings))
		}
		if !reflect.DeepEqual(updated.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", updated.Spec.RoleMappings, wantMappings)
		}
		if updated.Spec.Effect != openchoreov1alpha1.EffectDeny {
			t.Errorf("effect = %s, want deny", updated.Spec.Effect)
		}
	})

	t.Run("update cluster role binding with multiple role mappings", func(t *testing.T) {
		// Create with a single mapping first
		binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "update-multi-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "team"},
				RoleMappings: []openchoreov1alpha1.ClusterRoleMapping{
					{RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "initial-role"}},
				},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}

		// Update to multiple mappings and change effect
		wantMappings := []openchoreov1alpha1.ClusterRoleMapping{
			{RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "role-a"}},
			{RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "role-b"}},
		}
		updated, err := enforcer.UpdateClusterRoleBinding(ctx, &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "update-multi-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "team"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectDeny,
			},
		})
		if err != nil {
			t.Fatalf("UpdateClusterRoleBinding() error = %v", err)
		}
		if len(updated.Spec.RoleMappings) != len(wantMappings) {
			t.Fatalf("RoleMappings length = %d, want %d", len(updated.Spec.RoleMappings), len(wantMappings))
		}
		if !reflect.DeepEqual(updated.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", updated.Spec.RoleMappings, wantMappings)
		}
		if updated.Spec.Effect != openchoreov1alpha1.EffectDeny {
			t.Errorf("effect = %s, want deny", updated.Spec.Effect)
		}

		// Verify round-trip via Get
		fetched, err := enforcer.GetClusterRoleBinding(ctx, "update-multi-crb")
		if err != nil {
			t.Fatalf("GetClusterRoleBinding() error = %v", err)
		}
		if len(fetched.Spec.RoleMappings) != len(wantMappings) {
			t.Fatalf("fetched RoleMappings length = %d, want %d", len(fetched.Spec.RoleMappings), len(wantMappings))
		}
		if !reflect.DeepEqual(fetched.Spec.RoleMappings, wantMappings) {
			t.Errorf("fetched RoleMappings = %v, want %v", fetched.Spec.RoleMappings, wantMappings)
		}
		if fetched.Spec.Effect != openchoreov1alpha1.EffectDeny {
			t.Errorf("fetched effect = %s, want deny", fetched.Spec.Effect)
		}
	})

	t.Run("update cluster role binding to add scope", func(t *testing.T) {
		binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "update-scope-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "scope-team"},
				RoleMappings: []openchoreov1alpha1.ClusterRoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "viewer"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}

		wantMappings := []openchoreov1alpha1.ClusterRoleMapping{{
			RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "viewer"},
			Scope:   openchoreov1alpha1.ClusterTargetScope{Namespace: "acme", Project: "p1"},
		}}
		updated, err := enforcer.UpdateClusterRoleBinding(ctx, &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "update-scope-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "scope-team"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectAllow,
			},
		})
		if err != nil {
			t.Fatalf("UpdateClusterRoleBinding() error = %v", err)
		}
		if !reflect.DeepEqual(updated.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", updated.Spec.RoleMappings, wantMappings)
		}

		// Verify round-trip
		fetched, err := enforcer.GetClusterRoleBinding(ctx, "update-scope-crb")
		if err != nil {
			t.Fatalf("GetClusterRoleBinding() error = %v", err)
		}
		if !reflect.DeepEqual(fetched.Spec.RoleMappings, wantMappings) {
			t.Errorf("fetched RoleMappings = %v, want %v", fetched.Spec.RoleMappings, wantMappings)
		}
	})

	t.Run("update non-existent cluster role binding", func(t *testing.T) {
		_, err := enforcer.UpdateClusterRoleBinding(ctx, &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "no-exist-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "g"},
				RoleMappings: []openchoreov1alpha1.ClusterRoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "r"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		})
		if !errors.Is(err, authzcore.ErrRoleMappingNotFound) {
			t.Errorf("error = %v, want ErrRoleMappingNotFound", err)
		}
	})

	t.Run("update nil cluster role binding", func(t *testing.T) {
		_, err := enforcer.UpdateClusterRoleBinding(ctx, nil)
		if !errors.Is(err, authzcore.ErrInvalidRequest) {
			t.Errorf("error = %v, want ErrInvalidRequest", err)
		}
	})
}

// ============================================================================
// PAP Tests - CRD-returning Namespaced Role Binding Methods
// ============================================================================

func TestCasbinEnforcer_CreateNamespacedRoleBinding(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	t.Run("create namespaced role binding with single mapping", func(t *testing.T) {
		wantMappings := []openchoreov1alpha1.RoleMapping{{
			RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "dev-role"},
			Scope:   openchoreov1alpha1.TargetScope{Project: "p1"},
		}}
		binding := &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "crd-rb-1", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "devs"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectAllow,
			},
		}
		created, err := enforcer.CreateNamespacedRoleBinding(ctx, binding)
		if err != nil {
			t.Fatalf("CreateNamespacedRoleBinding() error = %v", err)
		}
		if created.Name != "crd-rb-1" || created.Namespace != testNs {
			t.Errorf("result = %s/%s, want %s/crd-rb-1", created.Namespace, created.Name, testNs)
		}
		if len(created.Spec.RoleMappings) != 1 {
			t.Fatalf("RoleMappings length = %d, want 1", len(created.Spec.RoleMappings))
		}
		if !reflect.DeepEqual(created.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", created.Spec.RoleMappings, wantMappings)
		}
		if created.Spec.Effect != openchoreov1alpha1.EffectAllow {
			t.Errorf("effect = %s, want allow", created.Spec.Effect)
		}
	})

	t.Run("create namespaced role binding with multiple mappings", func(t *testing.T) {
		wantMappings := []openchoreov1alpha1.RoleMapping{
			{
				RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "viewer-role"},
				Scope:   openchoreov1alpha1.TargetScope{Project: "proj-a"},
			},
			{
				RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "admin-role"},
				Scope:   openchoreov1alpha1.TargetScope{Project: "proj-b", Component: "comp-1"},
			},
			{
				RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "editor-role"},
			},
		}
		binding := &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "multi-rb", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "multi-devs"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectAllow,
			},
		}
		created, err := enforcer.CreateNamespacedRoleBinding(ctx, binding)
		if err != nil {
			t.Fatalf("CreateNamespacedRoleBinding() error = %v", err)
		}
		if len(created.Spec.RoleMappings) != len(wantMappings) {
			t.Fatalf("RoleMappings length = %d, want %d", len(created.Spec.RoleMappings), len(wantMappings))
		}
		if !reflect.DeepEqual(created.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", created.Spec.RoleMappings, wantMappings)
		}
		if created.Spec.Effect != openchoreov1alpha1.EffectAllow {
			t.Errorf("effect = %s, want allow", created.Spec.Effect)
		}

		// Verify round-trip via Get
		fetched, err := enforcer.GetNamespacedRoleBinding(ctx, "multi-rb", testNs)
		if err != nil {
			t.Fatalf("GetNamespacedRoleBinding() error = %v", err)
		}
		if len(fetched.Spec.RoleMappings) != len(wantMappings) {
			t.Fatalf("fetched RoleMappings length = %d, want %d", len(fetched.Spec.RoleMappings), len(wantMappings))
		}
		if !reflect.DeepEqual(fetched.Spec.RoleMappings, wantMappings) {
			t.Errorf("fetched RoleMappings = %v, want %v", fetched.Spec.RoleMappings, wantMappings)
		}
	})

	t.Run("create duplicate namespaced role binding", func(t *testing.T) {
		binding := &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "dup-rb", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "g"},
				RoleMappings: []openchoreov1alpha1.RoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "r"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		}
		if _, err := enforcer.CreateNamespacedRoleBinding(ctx, binding); err != nil {
			t.Fatalf("first call error = %v", err)
		}
		_, err := enforcer.CreateNamespacedRoleBinding(ctx, &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "dup-rb", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "g"},
				RoleMappings: []openchoreov1alpha1.RoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "r"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		})
		if !errors.Is(err, authzcore.ErrRoleMappingAlreadyExists) {
			t.Errorf("duplicate error = %v, want ErrRoleMappingAlreadyExists", err)
		}
	})

	t.Run("create nil namespaced role binding", func(t *testing.T) {
		_, err := enforcer.CreateNamespacedRoleBinding(ctx, nil)
		if !errors.Is(err, authzcore.ErrInvalidRequest) {
			t.Errorf("error = %v, want ErrInvalidRequest", err)
		}
	})
}

func TestCasbinEnforcer_GetNamespacedRoleBinding(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	t.Run("get existing with single mapping", func(t *testing.T) {
		wantMappings := []openchoreov1alpha1.RoleMapping{{
			RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "ops-role"},
		}}
		binding := &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "get-rb", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "ops"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectAllow,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}

		fetched, err := enforcer.GetNamespacedRoleBinding(ctx, "get-rb", testNs)
		if err != nil {
			t.Fatalf("GetNamespacedRoleBinding() error = %v", err)
		}
		if len(fetched.Spec.RoleMappings) != 1 {
			t.Fatalf("RoleMappings length = %d, want 1", len(fetched.Spec.RoleMappings))
		}
		if !reflect.DeepEqual(fetched.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", fetched.Spec.RoleMappings, wantMappings)
		}
		if fetched.Spec.Effect != openchoreov1alpha1.EffectAllow {
			t.Errorf("effect = %s, want allow", fetched.Spec.Effect)
		}
	})

	t.Run("get existing with multiple mappings", func(t *testing.T) {
		wantMappings := []openchoreov1alpha1.RoleMapping{
			{
				RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "role-a"},
				Scope:   openchoreov1alpha1.TargetScope{Project: "proj-1"},
			},
			{
				RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "role-b"},
				Scope:   openchoreov1alpha1.TargetScope{Project: "proj-2", Component: "comp-x"},
			},
		}
		binding := &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "get-multi-rb", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "multi-ops"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectDeny,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}

		fetched, err := enforcer.GetNamespacedRoleBinding(ctx, "get-multi-rb", testNs)
		if err != nil {
			t.Fatalf("GetNamespacedRoleBinding() error = %v", err)
		}
		if len(fetched.Spec.RoleMappings) != len(wantMappings) {
			t.Fatalf("RoleMappings length = %d, want %d", len(fetched.Spec.RoleMappings), len(wantMappings))
		}
		if !reflect.DeepEqual(fetched.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", fetched.Spec.RoleMappings, wantMappings)
		}
		if fetched.Spec.Effect != openchoreov1alpha1.EffectDeny {
			t.Errorf("effect = %s, want deny", fetched.Spec.Effect)
		}
	})

	t.Run("get non-existent", func(t *testing.T) {
		_, err := enforcer.GetNamespacedRoleBinding(ctx, "non-existent", testNs)
		if !errors.Is(err, authzcore.ErrRoleMappingNotFound) {
			t.Errorf("error = %v, want ErrRoleMappingNotFound", err)
		}
	})

	t.Run("get wrong namespace", func(t *testing.T) {
		_, err := enforcer.GetNamespacedRoleBinding(ctx, "get-rb", "wrong-ns")
		if !errors.Is(err, authzcore.ErrRoleMappingNotFound) {
			t.Errorf("error = %v, want ErrRoleMappingNotFound", err)
		}
	})
}

func TestCasbinEnforcer_ListNamespacedRoleBindings(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	for _, r := range []struct {
		name string
		ns   string
	}{
		{"list-rb-1", testNs},
		{"list-rb-2", testNs},
		{"list-rb-other", "other-ns"},
	} {
		binding := &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: r.name, Namespace: r.ns},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "g"},
				RoleMappings: []openchoreov1alpha1.RoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "r"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	t.Run("list bindings in specific namespace", func(t *testing.T) {
		list, err := enforcer.ListNamespacedRoleBindings(ctx, testNs, 0, "")
		if err != nil {
			t.Fatalf("ListNamespacedRoleBindings() error = %v", err)
		}
		if len(list.Items) != 2 {
			t.Errorf("returned %d items, want 2", len(list.Items))
		}
		for _, item := range list.Items {
			if item.Namespace != testNs {
				t.Errorf("item namespace = %s, want %s", item.Namespace, testNs)
			}
		}
	})

	t.Run("list bindings in empty namespace", func(t *testing.T) {
		list, err := enforcer.ListNamespacedRoleBindings(ctx, "empty-ns", 0, "")
		if err != nil {
			t.Fatalf("ListNamespacedRoleBindings() error = %v", err)
		}
		if len(list.Items) != 0 {
			t.Errorf("returned %d items, want 0", len(list.Items))
		}
	})
}

func TestCasbinEnforcer_UpdateNamespacedRoleBinding(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	t.Run("update existing namespaced role binding with single mapping", func(t *testing.T) {
		binding := &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "update-rb", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "old-group"},
				RoleMappings: []openchoreov1alpha1.RoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "old-role"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}

		wantMappings := []openchoreov1alpha1.RoleMapping{{
			RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "new-role"},
			Scope:   openchoreov1alpha1.TargetScope{Project: "p1", Component: "c1"},
		}}
		updated, err := enforcer.UpdateNamespacedRoleBinding(ctx, &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "update-rb", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "new-group"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectDeny,
			},
		})
		if err != nil {
			t.Fatalf("UpdateNamespacedRoleBinding() error = %v", err)
		}
		if updated.Spec.Entitlement.Value != "new-group" {
			t.Errorf("entitlement value = %s, want new-group", updated.Spec.Entitlement.Value)
		}
		if len(updated.Spec.RoleMappings) != 1 {
			t.Fatalf("RoleMappings length = %d, want 1", len(updated.Spec.RoleMappings))
		}
		if !reflect.DeepEqual(updated.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", updated.Spec.RoleMappings, wantMappings)
		}
		if updated.Spec.Effect != openchoreov1alpha1.EffectDeny {
			t.Errorf("effect = %s, want deny", updated.Spec.Effect)
		}
	})

	t.Run("update namespaced role binding with multiple mappings", func(t *testing.T) {
		// Create with a single mapping
		binding := &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "update-multi-rb", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "team"},
				RoleMappings: []openchoreov1alpha1.RoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "initial"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}

		// Update to multiple mappings and change effect
		wantMappings := []openchoreov1alpha1.RoleMapping{
			{
				RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "role-a"},
				Scope:   openchoreov1alpha1.TargetScope{Project: "proj-1"},
			},
			{
				RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "role-b"},
				Scope:   openchoreov1alpha1.TargetScope{Project: "proj-2", Component: "comp-1"},
			},
		}
		updated, err := enforcer.UpdateNamespacedRoleBinding(ctx, &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "update-multi-rb", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "team"},
				RoleMappings: wantMappings,
				Effect:       openchoreov1alpha1.EffectDeny,
			},
		})
		if err != nil {
			t.Fatalf("UpdateNamespacedRoleBinding() error = %v", err)
		}
		if len(updated.Spec.RoleMappings) != len(wantMappings) {
			t.Fatalf("RoleMappings length = %d, want %d", len(updated.Spec.RoleMappings), len(wantMappings))
		}
		if !reflect.DeepEqual(updated.Spec.RoleMappings, wantMappings) {
			t.Errorf("RoleMappings = %v, want %v", updated.Spec.RoleMappings, wantMappings)
		}
		if updated.Spec.Effect != openchoreov1alpha1.EffectDeny {
			t.Errorf("effect = %s, want deny", updated.Spec.Effect)
		}

		// Verify round-trip via Get
		fetched, err := enforcer.GetNamespacedRoleBinding(ctx, "update-multi-rb", testNs)
		if err != nil {
			t.Fatalf("GetNamespacedRoleBinding() error = %v", err)
		}
		if len(fetched.Spec.RoleMappings) != len(wantMappings) {
			t.Fatalf("fetched RoleMappings length = %d, want %d", len(fetched.Spec.RoleMappings), len(wantMappings))
		}
		if !reflect.DeepEqual(fetched.Spec.RoleMappings, wantMappings) {
			t.Errorf("fetched RoleMappings = %v, want %v", fetched.Spec.RoleMappings, wantMappings)
		}
		if fetched.Spec.Effect != openchoreov1alpha1.EffectDeny {
			t.Errorf("fetched effect = %s, want deny", fetched.Spec.Effect)
		}
	})

	t.Run("update non-existent namespaced role binding", func(t *testing.T) {
		_, err := enforcer.UpdateNamespacedRoleBinding(ctx, &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "no-exist-rb", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "g"},
				RoleMappings: []openchoreov1alpha1.RoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "r"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		})
		if !errors.Is(err, authzcore.ErrRoleMappingNotFound) {
			t.Errorf("error = %v, want ErrRoleMappingNotFound", err)
		}
	})

	t.Run("update nil namespaced role binding", func(t *testing.T) {
		_, err := enforcer.UpdateNamespacedRoleBinding(ctx, nil)
		if !errors.Is(err, authzcore.ErrInvalidRequest) {
			t.Errorf("error = %v, want ErrInvalidRequest", err)
		}
	})
}

// TestCasbinEnforcer_DeleteClusterRoleBinding tests deleting cluster-scoped role bindings
func TestCasbinEnforcer_DeleteClusterRoleBinding(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("delete existing cluster role binding", func(t *testing.T) {
		binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "delete-crb"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "ops"},
				RoleMappings: []openchoreov1alpha1.ClusterRoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}

		if err := enforcer.DeleteClusterRoleBinding(ctx, "delete-crb"); err != nil {
			t.Fatalf("DeleteClusterRoleBinding() error = %v", err)
		}

		// Verify the CRD is gone
		var crd openchoreov1alpha1.ClusterAuthzRoleBinding
		err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "delete-crb"}, &crd)
		if !k8serrors.IsNotFound(err) {
			t.Errorf("expected NotFound after delete, got err = %v", err)
		}
	})

	t.Run("delete non-existent cluster role binding", func(t *testing.T) {
		err := enforcer.DeleteClusterRoleBinding(ctx, "non-existent-crb")
		if !errors.Is(err, authzcore.ErrRoleMappingNotFound) {
			t.Errorf("DeleteClusterRoleBinding() error = %v, want ErrRoleMappingNotFound", err)
		}
	})
}

// TestCasbinEnforcer_DeleteNamespacedRoleBinding tests deleting namespace-scoped role bindings
func TestCasbinEnforcer_DeleteNamespacedRoleBinding(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	t.Run("delete existing namespaced role binding", func(t *testing.T) {
		binding := &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "delete-rb", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "devs"},
				RoleMappings: []openchoreov1alpha1.RoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "dev-role"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}

		if err := enforcer.DeleteNamespacedRoleBinding(ctx, "delete-rb", testNs); err != nil {
			t.Fatalf("DeleteNamespacedRoleBinding() error = %v", err)
		}

		// Verify the CRD is gone
		var crd openchoreov1alpha1.AuthzRoleBinding
		err := enforcer.k8sClient.Get(ctx, client.ObjectKey{Name: "delete-rb", Namespace: testNs}, &crd)
		if !k8serrors.IsNotFound(err) {
			t.Errorf("expected NotFound after delete, got err = %v", err)
		}
	})

	t.Run("delete non-existent namespaced role binding", func(t *testing.T) {
		err := enforcer.DeleteNamespacedRoleBinding(ctx, "non-existent-rb", testNs)
		if !errors.Is(err, authzcore.ErrRoleMappingNotFound) {
			t.Errorf("DeleteNamespacedRoleBinding() error = %v, want ErrRoleMappingNotFound", err)
		}
	})

	t.Run("delete namespaced role binding wrong namespace", func(t *testing.T) {
		binding := &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "delete-wrong-ns-rb", Namespace: testNs},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: testClaimGroups, Value: "devs"},
				RoleMappings: []openchoreov1alpha1.RoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "dev-role"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		}
		if err := enforcer.k8sClient.Create(ctx, binding); err != nil {
			t.Fatalf("setup: %v", err)
		}

		err := enforcer.DeleteNamespacedRoleBinding(ctx, "delete-wrong-ns-rb", "wrong-ns")
		if !errors.Is(err, authzcore.ErrRoleMappingNotFound) {
			t.Errorf("DeleteNamespacedRoleBinding() error = %v, want ErrRoleMappingNotFound", err)
		}
	})
}

func TestCasbinEnforcer_ListClusterRoles_WithLimit(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// Seed 3 cluster roles
	for _, name := range []string{"limit-cr-1", "limit-cr-2", "limit-cr-3"} {
		_, err := enforcer.CreateClusterRole(ctx, &openchoreov1alpha1.ClusterAuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       openchoreov1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:view"}},
		})
		require.NoError(t, err, "CreateClusterRole(%s) error", name)
	}

	// First page: limit=2 from 3 items
	page1, err := enforcer.ListClusterRoles(ctx, 2, "")
	require.NoError(t, err, "ListClusterRoles(limit=2) error")
	require.NotNil(t, page1, "ListClusterRoles(limit=2) returned nil")
	// The fake k8s client may not enforce limit; accept either paginated (2 items + cursor) or all items at once
	if page1.NextCursor != "" {
		require.Len(t, page1.Items, 2, "when pagination cursor is returned, expected exactly 2 items on first page")
	}

	// If the client returns a cursor, use it to fetch the remaining item
	if page1.NextCursor != "" {
		page2, err := enforcer.ListClusterRoles(ctx, 2, page1.NextCursor)
		require.NoError(t, err, "ListClusterRoles(page2) error")
		require.Len(t, page2.Items, 1, "expected 1 item on second page")
		// Items across pages must not overlap
		seen := make(map[string]bool)
		for _, r := range page1.Items {
			seen[r.Name] = true
		}
		for _, r := range page2.Items {
			require.False(t, seen[r.Name], "item %q appeared on both pages", r.Name)
		}
	}
}

func TestCasbinEnforcer_ListClusterRoles_NoLimit(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// Seed 2 items so there's something to count
	for _, name := range []string{"nolimit-cr-1", "nolimit-cr-2"} {
		_, err := enforcer.CreateClusterRole(ctx, &openchoreov1alpha1.ClusterAuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       openchoreov1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:view"}},
		})
		require.NoError(t, err, "CreateClusterRole(%s) error", name)
	}

	result, err := enforcer.ListClusterRoles(ctx, 0, "")
	require.NoError(t, err, "ListClusterRoles(limit=0) error")
	require.GreaterOrEqual(t, len(result.Items), 2, "expected at least 2 items with no limit")
}

func TestCasbinEnforcer_ListClusterRoles_CursorWithoutLimit(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// cursor without limit should be ignored gracefully
	_, err := enforcer.ListClusterRoles(ctx, 0, "some-cursor")
	require.NoError(t, err, "ListClusterRoles(limit=0,cursor) error")
}

func TestCasbinEnforcer_ListNamespacedRoles_WithLimit(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	ns := "test-ns-list"

	for _, name := range []string{"limit-r-1", "limit-r-2"} {
		_, err := enforcer.CreateNamespacedRole(ctx, &openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       openchoreov1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
		})
		require.NoError(t, err, "CreateNamespacedRole(%s) error", name)
	}

	// First page: limit=1 from 2 items
	page1, err := enforcer.ListNamespacedRoles(ctx, ns, 1, "")
	require.NoError(t, err, "ListNamespacedRoles(limit=1) error")
	require.NotNil(t, page1, "ListNamespacedRoles(limit=1) returned nil")
	// The fake k8s client may not enforce limit; accept either paginated (1 item + cursor) or all items at once
	if page1.NextCursor != "" {
		require.Len(t, page1.Items, 1, "when pagination cursor is returned, expected exactly 1 item on first page")
	}

	// If the client returns a cursor, use it to fetch the second item
	if page1.NextCursor != "" {
		page2, err := enforcer.ListNamespacedRoles(ctx, ns, 1, page1.NextCursor)
		require.NoError(t, err, "ListNamespacedRoles(page2) error")
		require.Len(t, page2.Items, 1, "expected 1 item on second page")
		require.NotEqual(t, page1.Items[0].Name, page2.Items[0].Name, "same item appeared on both pages")
	}
}

func TestCasbinEnforcer_ListClusterRoleBindings_WithLimit(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	for _, name := range []string{"limit-crb-1", "limit-crb-2"} {
		_, err := enforcer.CreateClusterRoleBinding(ctx, &openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
				RoleMappings: []openchoreov1alpha1.ClusterRoleMapping{{
					RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"},
				}},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		})
		require.NoError(t, err, "CreateClusterRoleBinding(%s) error", name)
	}

	// First page: limit=1 from 2 items
	page1, err := enforcer.ListClusterRoleBindings(ctx, 1, "")
	require.NoError(t, err, "ListClusterRoleBindings(limit=1) error")
	require.NotNil(t, page1, "ListClusterRoleBindings(limit=1) returned nil")
	// The fake k8s client may not enforce limit; accept either paginated (1 item + cursor) or all items at once
	if page1.NextCursor != "" {
		require.Len(t, page1.Items, 1, "when pagination cursor is returned, expected exactly 1 item on first page")
	}

	// If the client returns a cursor, use it to fetch the second item
	if page1.NextCursor != "" {
		page2, err := enforcer.ListClusterRoleBindings(ctx, 1, page1.NextCursor)
		require.NoError(t, err, "ListClusterRoleBindings(page2) error")
		require.Len(t, page2.Items, 1, "expected 1 item on second page")
		require.NotEqual(t, page1.Items[0].Name, page2.Items[0].Name, "same item appeared on both pages")
	}
}

func TestCasbinEnforcer_ListNamespacedRoleBindings_WithLimit(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	ns := "test-ns-rb-list"

	for _, name := range []string{"limit-rb-1", "limit-rb-2"} {
		_, err := enforcer.CreateNamespacedRoleBinding(ctx, &openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement:  openchoreov1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
				RoleMappings: []openchoreov1alpha1.RoleMapping{{RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
				Effect:       openchoreov1alpha1.EffectAllow,
			},
		})
		require.NoError(t, err, "CreateNamespacedRoleBinding(%s) error", name)
	}

	// First page: limit=1 from 2 items
	page1, err := enforcer.ListNamespacedRoleBindings(ctx, ns, 1, "")
	require.NoError(t, err, "ListNamespacedRoleBindings(limit=1) error")
	require.NotNil(t, page1, "ListNamespacedRoleBindings(limit=1) returned nil")
	// The fake k8s client may not enforce limit; accept either paginated (1 item + cursor) or all items at once
	if page1.NextCursor != "" {
		require.Len(t, page1.Items, 1, "when pagination cursor is returned, expected exactly 1 item on first page")
	}

	// If the client returns a cursor, use it to fetch the second item
	if page1.NextCursor != "" {
		page2, err := enforcer.ListNamespacedRoleBindings(ctx, ns, 1, page1.NextCursor)
		require.NoError(t, err, "ListNamespacedRoleBindings(page2) error")
		require.Len(t, page2.Items, 1, "expected 1 item on second page")
		require.NotEqual(t, page1.Items[0].Name, page2.Items[0].Name, "same item appeared on both pages")
	}
}

func TestCasbinEnforcer_UpdateClusterRole_NotFound(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	_, err := enforcer.UpdateClusterRole(ctx, &openchoreov1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "non-existent-cr"},
		Spec:       openchoreov1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:view"}},
	})
	require.Error(t, err, "UpdateClusterRole non-existent should return error")
}

func TestCasbinEnforcer_UpdateNamespacedRole_NotFound(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	_, err := enforcer.UpdateNamespacedRole(ctx, &openchoreov1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "non-existent-r", Namespace: "acme"},
		Spec:       openchoreov1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
	})
	require.Error(t, err, "UpdateNamespacedRole non-existent should return error")
}

func TestCasbinEnforcer_UpdateClusterRoleBinding_NotFound(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	_, err := enforcer.UpdateClusterRoleBinding(ctx, &openchoreov1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "non-existent-crb"},
	})
	require.Error(t, err, "UpdateClusterRoleBinding non-existent should return error")
}

func TestCasbinEnforcer_UpdateNamespacedRoleBinding_NotFound(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	_, err := enforcer.UpdateNamespacedRoleBinding(ctx, &openchoreov1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "non-existent-rb", Namespace: "acme"},
	})
	require.Error(t, err, "UpdateNamespacedRoleBinding non-existent should return error")
}

func TestCasbinEnforcer_ListClusterRoles_WithCursorAndLimit(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	// Fake client doesn't enforce cursor but exercises the branch
	_, err := enforcer.ListClusterRoles(ctx, 1, "some-cursor-token")
	require.NoError(t, err, "ListClusterRoles(limit=1,cursor) error")
}

func TestCasbinEnforcer_ListNamespacedRoles_WithCursor(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	_, err := enforcer.ListNamespacedRoles(ctx, "acme", 0, "some-cursor-token")
	require.NoError(t, err, "ListNamespacedRoles(cursor) error")
}

func TestCasbinEnforcer_ListClusterRoleBindings_WithCursorAndLimit(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	_, err := enforcer.ListClusterRoleBindings(ctx, 1, "some-cursor-token")
	require.NoError(t, err, "ListClusterRoleBindings(limit=1,cursor) error")
}

func TestCasbinEnforcer_ListNamespacedRoleBindings_WithCursor(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	_, err := enforcer.ListNamespacedRoleBindings(ctx, "acme", 0, "some-cursor-token")
	require.NoError(t, err, "ListNamespacedRoleBindings(cursor) error")
}
