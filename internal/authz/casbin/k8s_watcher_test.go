// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"io"
	"log/slog"
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	authzv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// setupTestHandler creates a test handler with a fresh Casbin enforcer
func setupTestHandler(t *testing.T, crdType string) (*authzInformerHandler, casbin.IEnforcer) {
	t.Helper()

	m, err := model.NewModelFromString(embeddedModel)
	if err != nil {
		t.Fatalf("failed to load model: %v", err)
	}

	enforcer, err := casbin.NewSyncedEnforcer(m)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}

	// Register custom functions used by the model
	enforcer.AddFunction("resourceMatch", resourceMatchWrapper)
	enforcer.AddFunction("ctxMatch", ctxMatchWrapper)

	// Add custom role matcher for action wildcards
	enforcer.Enforcer.AddNamedMatchingFunc("g", "", roleActionMatchWrapper)

	handler := &authzInformerHandler{
		enforcer: enforcer,
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		crdType:  crdType,
	}

	return handler, enforcer
}

func TestAuthzInformerHandler_HandleAddRole(t *testing.T) {
	tests := []struct {
		name         string
		role         *authzv1alpha1.AuthzRole
		wantPolicies [][]string // Expected grouping policies: [role, action, namespace]
	}{
		{
			name: "add role with single action",
			role: &authzv1alpha1.AuthzRole{
				ObjectMeta: metav1.ObjectMeta{Name: "viewer", Namespace: "acme"},
				Spec: authzv1alpha1.AuthzRoleSpec{
					Actions: []string{"component:view"},
				},
			},
			wantPolicies: [][]string{
				{"viewer", "component:view", "acme"},
			},
		},
		{
			name: "add role with multiple actions",
			role: &authzv1alpha1.AuthzRole{
				ObjectMeta: metav1.ObjectMeta{Name: "editor", Namespace: "acme"},
				Spec: authzv1alpha1.AuthzRoleSpec{
					Actions: []string{"component:view", "component:create", "component:update"},
				},
			},
			wantPolicies: [][]string{
				{"editor", "component:view", "acme"},
				{"editor", "component:create", "acme"},
				{"editor", "component:update", "acme"},
			},
		},
		{
			name: "add role with wildcard action",
			role: &authzv1alpha1.AuthzRole{
				ObjectMeta: metav1.ObjectMeta{Name: "admin", Namespace: "acme"},
				Spec: authzv1alpha1.AuthzRoleSpec{
					Actions: []string{"component:*"},
				},
			},
			wantPolicies: [][]string{
				{"admin", "component:*", "acme"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, enforcer := setupTestHandler(t, CRDTypeAuthzRole)

			err := handler.handleAddRole(tt.role)

			if err != nil {
				t.Fatalf("handleAddRole() unexpected error = %v", err)
			}

			for _, expected := range tt.wantPolicies {
				hasPolicy, err := enforcer.HasGroupingPolicy(expected[0], expected[1], expected[2])
				if err != nil {
					t.Errorf("HasGroupingPolicy() error = %v", err)
					continue
				}
				if !hasPolicy {
					t.Errorf("expected grouping policy %v not found", expected)
				}
			}
		})
	}
}

func TestAuthzInformerHandler_HandleAddRole_Idempotent(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeAuthzRole)

	role := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "viewer", Namespace: "acme"},
		Spec: authzv1alpha1.AuthzRoleSpec{
			Actions: []string{"component:view"},
		},
	}

	// Add twice
	if err := handler.handleAddRole(role); err != nil {
		t.Fatalf("first handleAddRole() error = %v", err)
	}
	if err := handler.handleAddRole(role); err != nil {
		t.Fatalf("second handleAddRole() error = %v", err)
	}

	// Should still only have one policy
	policies, err := enforcer.GetFilteredGroupingPolicy(0, "viewer")
	if err != nil {
		t.Fatalf("GetFilteredGroupingPolicy() error = %v", err)
	}
	if len(policies) != 1 {
		t.Errorf("expected 1 policy, got %d", len(policies))
	}
}

func TestAuthzInformerHandler_HandleAddClusterRole(t *testing.T) {
	tests := []struct {
		name         string
		clusterRole  *authzv1alpha1.AuthzClusterRole
		wantPolicies [][]string // Expected grouping policies: [role, action, "*"]
	}{
		{
			name: "add cluster role with single action",
			clusterRole: &authzv1alpha1.AuthzClusterRole{
				ObjectMeta: metav1.ObjectMeta{Name: "global-viewer"},
				Spec: authzv1alpha1.AuthzClusterRoleSpec{
					Actions: []string{"namespace:view"},
				},
			},
			wantPolicies: [][]string{
				{"global-viewer", "namespace:view", "*"},
			},
		},
		{
			name: "add cluster role with multiple actions",
			clusterRole: &authzv1alpha1.AuthzClusterRole{
				ObjectMeta: metav1.ObjectMeta{Name: "global-admin"},
				Spec: authzv1alpha1.AuthzClusterRoleSpec{
					Actions: []string{"namespace:view", "namespace:create", "project:*"},
				},
			},
			wantPolicies: [][]string{
				{"global-admin", "namespace:view", "*"},
				{"global-admin", "namespace:create", "*"},
				{"global-admin", "project:*", "*"},
			},
		},
		{
			name: "add cluster role with global wildcard",
			clusterRole: &authzv1alpha1.AuthzClusterRole{
				ObjectMeta: metav1.ObjectMeta{Name: "super-admin"},
				Spec: authzv1alpha1.AuthzClusterRoleSpec{
					Actions: []string{"*"},
				},
			},
			wantPolicies: [][]string{
				{"super-admin", "*", "*"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, enforcer := setupTestHandler(t, CRDTypeAuthzClusterRole)

			err := handler.handleAddClusterRole(tt.clusterRole)
			if err != nil {
				t.Fatalf("handleAddClusterRole() error = %v", err)
			}

			for _, expected := range tt.wantPolicies {
				hasPolicy, err := enforcer.HasGroupingPolicy(expected[0], expected[1], expected[2])
				if err != nil {
					t.Errorf("HasGroupingPolicy() error = %v", err)
					continue
				}
				if !hasPolicy {
					t.Errorf("expected grouping policy %v not found", expected)
				}
			}
		})
	}
}

func TestAuthzInformerHandler_HandleAddClusterRole_Idempotent(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeAuthzClusterRole)

	clusterRole := &authzv1alpha1.AuthzClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "global-viewer"},
		Spec: authzv1alpha1.AuthzClusterRoleSpec{
			Actions: []string{"namespace:view"},
		},
	}

	// Add twice
	if err := handler.handleAddClusterRole(clusterRole); err != nil {
		t.Fatalf("first handleAddClusterRole() error = %v", err)
	}
	if err := handler.handleAddClusterRole(clusterRole); err != nil {
		t.Fatalf("second handleAddClusterRole() error = %v", err)
	}

	// Should still only have one policy
	policies, err := enforcer.GetFilteredGroupingPolicy(0, "global-viewer")
	if err != nil {
		t.Fatalf("GetFilteredGroupingPolicy() error = %v", err)
	}
	if len(policies) != 1 {
		t.Errorf("expected 1 policy, got %d", len(policies))
	}
}

func TestAuthzInformerHandler_HandleAddBinding(t *testing.T) {
	tests := []struct {
		name       string
		binding    *authzv1alpha1.AuthzRoleBinding
		wantPolicy []string // Expected policy: [subject, resource, role, namespace, effect, context, binding_name]
	}{
		{
			name: "add binding at namespace level",
			binding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: "acme"},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "developers",
					},
					RoleRef: authzv1alpha1.RoleRef{
						Kind: CRDTypeAuthzRole,
						Name: "editor",
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicy: []string{"groups:developers", "ns/acme", "editor", "acme", "allow", "{}", "dev-binding"},
		},
		{
			name: "add binding at project level",
			binding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "project-binding", Namespace: "acme"},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "project-team",
					},
					RoleRef: authzv1alpha1.RoleRef{
						Kind: CRDTypeAuthzRole,
						Name: "viewer",
					},
					TargetPath: authzv1alpha1.TargetPath{
						Project: "my-project",
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicy: []string{"groups:project-team", "ns/acme/project/my-project", "viewer", "acme", "allow", "{}", "project-binding"},
		},
		{
			name: "add binding at component level",
			binding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "component-binding", Namespace: "acme"},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "sub",
						Value: "user-123",
					},
					RoleRef: authzv1alpha1.RoleRef{
						Kind: CRDTypeAuthzRole,
						Name: "deployer",
					},
					TargetPath: authzv1alpha1.TargetPath{
						Project:   "my-project",
						Component: "my-component",
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicy: []string{"sub:user-123", "ns/acme/project/my-project/component/my-component", "deployer", "acme", "allow", "{}", "component-binding"},
		},
		{
			name: "add deny binding",
			binding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "deny-binding", Namespace: "acme"},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "restricted-group",
					},
					RoleRef: authzv1alpha1.RoleRef{
						Kind: CRDTypeAuthzRole,
						Name: "editor",
					},
					TargetPath: authzv1alpha1.TargetPath{
						Project: "secret-project",
					},
					Effect: authzv1alpha1.EffectDeny,
				},
			},
			wantPolicy: []string{"groups:restricted-group", "ns/acme/project/secret-project", "editor", "acme", "deny", "{}", "deny-binding"},
		},
		{
			name: "add binding referencing cluster role",
			binding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-role-binding", Namespace: "acme"},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "admins",
					},
					RoleRef: authzv1alpha1.RoleRef{
						Kind: CRDTypeAuthzClusterRole,
						Name: "global-admin",
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicy: []string{"groups:admins", "ns/acme", "global-admin", "*", "allow", "{}", "cluster-role-binding"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, enforcer := setupTestHandler(t, CRDTypeAuthzRoleBinding)

			err := handler.handleAddBinding(tt.binding)
			if err != nil {
				t.Fatalf("handleAddBinding() error = %v", err)
			}

			hasPolicy, err := enforcer.HasPolicy(tt.wantPolicy[0], tt.wantPolicy[1], tt.wantPolicy[2], tt.wantPolicy[3], tt.wantPolicy[4], tt.wantPolicy[5], tt.wantPolicy[6])
			if err != nil {
				t.Fatalf("HasPolicy() error = %v", err)
			}
			if !hasPolicy {
				policies, _ := enforcer.GetPolicy()
				t.Errorf("expected policy %v not found. All policies: %v", tt.wantPolicy, policies)
			}
		})
	}
}

func TestAuthzInformerHandler_HandleAddBinding_Errors(t *testing.T) {
	tests := []struct {
		name           string
		binding        *authzv1alpha1.AuthzRoleBinding
		wantErrContain string
	}{
		{
			name: "missing effect",
			binding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "no-effect-binding", Namespace: "acme"},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "developers",
					},
					RoleRef: authzv1alpha1.RoleRef{
						Kind: CRDTypeAuthzRole,
						Name: "editor",
					},
				},
			},
			wantErrContain: "effect not specified",
		},
		{
			name: "missing subject",
			binding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "no-value-binding", Namespace: "acme"},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "",
					},
					RoleRef: authzv1alpha1.RoleRef{
						Kind: CRDTypeAuthzRole,
						Name: "editor",
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)

			err := handler.handleAddBinding(tt.binding)
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErrContain)
				return
			}
		})
	}
}

func TestAuthzInformerHandler_HandleAddBinding_Idempotent(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeAuthzRoleBinding)

	binding := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: "acme"},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{
				Claim: "groups",
				Value: "developers",
			},
			RoleRef: authzv1alpha1.RoleRef{
				Kind: CRDTypeAuthzRole,
				Name: "editor",
			},
			Effect: authzv1alpha1.EffectAllow,
		},
	}

	// Add twice
	if err := handler.handleAddBinding(binding); err != nil {
		t.Fatalf("first handleAddBinding() error = %v", err)
	}
	if err := handler.handleAddBinding(binding); err != nil {
		t.Fatalf("second handleAddBinding() error = %v", err)
	}

	// Should still only have one policy
	policies, err := enforcer.GetPolicy()
	if err != nil {
		t.Fatalf("GetPolicy() error = %v", err)
	}
	if len(policies) != 1 {
		t.Errorf("expected 1 policy, got %d", len(policies))
	}
}

func TestAuthzInformerHandler_HandleAddClusterBinding(t *testing.T) {
	tests := []struct {
		name       string
		binding    *authzv1alpha1.AuthzClusterRoleBinding
		wantPolicy []string // Expected policy: [subject, "*", role, "*", effect, context, binding_name]
	}{
		{
			name: "add cluster binding with allow effect",
			binding: &authzv1alpha1.AuthzClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "global-admin-binding"},
				Spec: authzv1alpha1.AuthzClusterRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "platform-admins",
					},
					RoleRef: authzv1alpha1.RoleRef{
						Kind: CRDTypeAuthzClusterRole,
						Name: "super-admin",
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicy: []string{"groups:platform-admins", "*", "super-admin", "*", "allow", "{}", "global-admin-binding"},
		},
		{
			name: "add cluster binding with deny effect",
			binding: &authzv1alpha1.AuthzClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "global-deny-binding"},
				Spec: authzv1alpha1.AuthzClusterRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "blocked-group",
					},
					RoleRef: authzv1alpha1.RoleRef{
						Kind: CRDTypeAuthzClusterRole,
						Name: "all-access",
					},
					Effect: authzv1alpha1.EffectDeny,
				},
			},
			wantPolicy: []string{"groups:blocked-group", "*", "all-access", "*", "deny", "{}", "global-deny-binding"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, enforcer := setupTestHandler(t, CRDTypeAuthzClusterRoleBinding)

			err := handler.handleAddClusterBinding(tt.binding)
			if err != nil {
				t.Fatalf("handleAddClusterBinding() error = %v", err)
			}

			hasPolicy, err := enforcer.HasPolicy(tt.wantPolicy[0], tt.wantPolicy[1], tt.wantPolicy[2], tt.wantPolicy[3], tt.wantPolicy[4], tt.wantPolicy[5], tt.wantPolicy[6])
			if err != nil {
				t.Fatalf("HasPolicy() error = %v", err)
			}
			if !hasPolicy {
				policies, _ := enforcer.GetPolicy()
				t.Errorf("expected policy %v not found. All policies: %v", tt.wantPolicy, policies)
			}
		})
	}
}

func TestAuthzInformerHandler_HandleAddClusterBinding_Idempotent(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeAuthzClusterRoleBinding)

	binding := &authzv1alpha1.AuthzClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "global-admin-binding"},
		Spec: authzv1alpha1.AuthzClusterRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{
				Claim: "groups",
				Value: "platform-admins",
			},
			RoleRef: authzv1alpha1.RoleRef{
				Kind: CRDTypeAuthzClusterRole,
				Name: "super-admin",
			},
			Effect: authzv1alpha1.EffectAllow,
		},
	}

	// Add twice
	if err := handler.handleAddClusterBinding(binding); err != nil {
		t.Fatalf("first handleAddClusterBinding() error = %v", err)
	}
	if err := handler.handleAddClusterBinding(binding); err != nil {
		t.Fatalf("second handleAddClusterBinding() error = %v", err)
	}

	// Should still only have one policy
	policies, err := enforcer.GetPolicy()
	if err != nil {
		t.Fatalf("GetPolicy() error = %v", err)
	}
	if len(policies) != 1 {
		t.Errorf("expected 1 policy, got %d", len(policies))
	}
}

// ============================================================================
// Update Handler Tests
// ============================================================================

func TestAuthzInformerHandler_HandleUpdateRole(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeAuthzRole)

	_, err := enforcer.AddGroupingPolicies([][]string{
		{"editor", "component:view", "acme"},
		{"editor", "component:create", "acme"},
	})
	if err != nil {
		t.Fatalf("AddGroupingPolicies() error = %v", err)
	}

	oldRole := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "editor",
			Namespace:  "acme",
			Generation: 1,
		},
		Spec: authzv1alpha1.AuthzRoleSpec{
			Actions: []string{"component:view", "component:create"},
		},
	}

	newRole := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "editor",
			Namespace:  "acme",
			Generation: 2, // Generation changed
		},
		Spec: authzv1alpha1.AuthzRoleSpec{
			Actions: []string{"component:view", "component:update", "component:delete"}, // create removed, update/delete added
		},
	}

	if err := handler.handleUpdateRole(oldRole, newRole); err != nil {
		t.Fatalf("handleUpdateRole() error = %v", err)
	}

	// Verify: view should remain, create removed, update/delete added
	hasView, _ := enforcer.HasGroupingPolicy("editor", "component:view", "acme")
	hasCreate, _ := enforcer.HasGroupingPolicy("editor", "component:create", "acme")
	hasUpdate, _ := enforcer.HasGroupingPolicy("editor", "component:update", "acme")
	hasDelete, _ := enforcer.HasGroupingPolicy("editor", "component:delete", "acme")

	if !hasView {
		t.Error("component:view should still exist")
	}
	if hasCreate {
		t.Error("component:create should be removed")
	}
	if !hasUpdate {
		t.Error("component:update should be added")
	}
	if !hasDelete {
		t.Error("component:delete should be added")
	}
}

func TestAuthzInformerHandler_HandleUpdateRole_NoGenerationChange(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeAuthzRole)

	_, err := enforcer.AddGroupingPolicy("editor", "component:view", "acme")
	if err != nil {
		t.Fatalf("AddGroupingPolicy() error = %v", err)
	}

	oldRole := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "editor",
			Namespace:  "acme",
			Generation: 1,
		},
		Spec: authzv1alpha1.AuthzRoleSpec{
			Actions: []string{"component:view"},
		},
	}

	// Same generation - should skip update
	newRole := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "editor",
			Namespace:  "acme",
			Generation: 1,
		},
		Spec: authzv1alpha1.AuthzRoleSpec{
			Actions: []string{"component:delete"}, // Different actions but same generation
		},
	}

	if err := handler.handleUpdateRole(oldRole, newRole); err != nil {
		t.Fatalf("handleUpdateRole() error = %v", err)
	}

	hasView, _ := enforcer.HasGroupingPolicy("editor", "component:view", "acme")
	hasDelete, _ := enforcer.HasGroupingPolicy("editor", "component:delete", "acme")

	if !hasView {
		t.Error("component:view should still exist (update skipped)")
	}
	if hasDelete {
		t.Error("component:delete should not be added (update skipped)")
	}
}

func TestAuthzInformerHandler_HandleUpdateClusterRole(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeAuthzClusterRole)

	_, err := enforcer.AddGroupingPolicies([][]string{
		{"global-editor", "namespace:view", "*"},
		{"global-editor", "namespace:create", "*"},
	})
	if err != nil {
		t.Fatalf("AddGroupingPolicies() error = %v", err)
	}

	oldRole := &authzv1alpha1.AuthzClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "global-editor",
			Generation: 1,
		},
		Spec: authzv1alpha1.AuthzClusterRoleSpec{
			Actions: []string{"namespace:view", "namespace:create"},
		},
	}

	newRole := &authzv1alpha1.AuthzClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "global-editor",
			Generation: 2,
		},
		Spec: authzv1alpha1.AuthzClusterRoleSpec{
			Actions: []string{"namespace:view", "namespace:delete"}, // create removed, delete added
		},
	}

	// Test: update handler
	if err := handler.handleUpdateClusterRole(oldRole, newRole); err != nil {
		t.Fatalf("handleUpdateClusterRole() error = %v", err)
	}

	hasView, _ := enforcer.HasGroupingPolicy("global-editor", "namespace:view", "*")
	hasCreate, _ := enforcer.HasGroupingPolicy("global-editor", "namespace:create", "*")
	hasDelete, _ := enforcer.HasGroupingPolicy("global-editor", "namespace:delete", "*")

	if !hasView {
		t.Error("namespace:view should still exist")
	}
	if hasCreate {
		t.Error("namespace:create should be removed")
	}
	if !hasDelete {
		t.Error("namespace:delete should be added")
	}
}

func TestAuthzInformerHandler_HandleDeleteRole(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeAuthzRole)

	_, err := enforcer.AddGroupingPolicies([][]string{
		{"editor", "component:view", "acme"},
		{"editor", "component:create", "acme"},
	})
	if err != nil {
		t.Fatalf("AddGroupingPolicies() error = %v", err)
	}

	role := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "editor", Namespace: "acme"},
		Spec: authzv1alpha1.AuthzRoleSpec{
			Actions: []string{"component:view", "component:create"},
		},
	}

	if err := handler.handleDeleteRole(role); err != nil {
		t.Fatalf("handleDeleteRole() error = %v", err)
	}

	hasView, _ := enforcer.HasGroupingPolicy("editor", "component:view", "acme")
	hasCreate, _ := enforcer.HasGroupingPolicy("editor", "component:create", "acme")

	if hasView || hasCreate {
		t.Error("all policies should be removed after delete")
	}
}

func TestAuthzInformerHandler_HandleDeleteClusterRole(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeAuthzClusterRole)

	_, err := enforcer.AddGroupingPolicies([][]string{
		{"global-admin", "namespace:view", "*"},
		{"global-admin", "namespace:create", "*"},
	})
	if err != nil {
		t.Fatalf("AddGroupingPolicies() error = %v", err)
	}

	clusterRole := &authzv1alpha1.AuthzClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "global-admin"},
		Spec: authzv1alpha1.AuthzClusterRoleSpec{
			Actions: []string{"namespace:view", "namespace:create"},
		},
	}

	if err := handler.handleDeleteClusterRole(clusterRole); err != nil {
		t.Fatalf("handleDeleteClusterRole() error = %v", err)
	}

	hasView, _ := enforcer.HasGroupingPolicy("global-admin", "namespace:view", "*")
	hasCreate, _ := enforcer.HasGroupingPolicy("global-admin", "namespace:create", "*")

	if hasView || hasCreate {
		t.Error("all policies should be removed after delete")
	}
}

func TestAuthzInformerHandler_HandleDeleteBinding(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeAuthzRoleBinding)

	_, err := enforcer.AddPolicy("groups:developers", "ns/acme", "editor", "acme", "allow", "{}", "dev-binding")
	if err != nil {
		t.Fatalf("AddPolicy() error = %v", err)
	}

	binding := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: "acme"},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{
				Claim: "groups",
				Value: "developers",
			},
			RoleRef: authzv1alpha1.RoleRef{
				Kind: CRDTypeAuthzRole,
				Name: "editor",
			},
			Effect: authzv1alpha1.EffectAllow,
		},
	}

	if err := handler.handleDeleteBinding(binding); err != nil {
		t.Fatalf("handleDeleteBinding() error = %v", err)
	}

	hasPolicy, _ := enforcer.HasPolicy("groups:developers", "ns/acme", "editor", "acme", "allow", "{}", "dev-binding")
	if hasPolicy {
		t.Error("policy should be removed after delete")
	}
}

func TestAuthzInformerHandler_HandleDeleteClusterBinding(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeAuthzClusterRoleBinding)

	// Setup: directly add policy to Casbin
	_, err := enforcer.AddPolicy("groups:platform-admins", "*", "super-admin", "*", "allow", "{}", "global-admin-binding")
	if err != nil {
		t.Fatalf("AddPolicy() error = %v", err)
	}

	binding := &authzv1alpha1.AuthzClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "global-admin-binding"},
		Spec: authzv1alpha1.AuthzClusterRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{
				Claim: "groups",
				Value: "platform-admins",
			},
			RoleRef: authzv1alpha1.RoleRef{
				Kind: CRDTypeAuthzClusterRole,
				Name: "super-admin",
			},
			Effect: authzv1alpha1.EffectAllow,
		},
	}

	if err := handler.handleDeleteClusterBinding(binding); err != nil {
		t.Fatalf("handleDeleteClusterBinding() error = %v", err)
	}

	hasPolicy, _ := enforcer.HasPolicy("groups:platform-admins", "*", "super-admin", "*", "allow", "{}", "global-admin-binding")
	if hasPolicy {
		t.Error("policy should be removed after delete")
	}
}
