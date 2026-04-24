// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"bytes"
	"io"
	"log/slog"
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

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
	enforcer.AddFunction("condMatch", condMatchWrapper)

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
		clusterRole  *authzv1alpha1.ClusterAuthzRole
		wantPolicies [][]string // Expected grouping policies: [role, action, "*"]
	}{
		{
			name: "add cluster role with single action",
			clusterRole: &authzv1alpha1.ClusterAuthzRole{
				ObjectMeta: metav1.ObjectMeta{Name: "global-viewer"},
				Spec: authzv1alpha1.ClusterAuthzRoleSpec{
					Actions: []string{"namespace:view"},
				},
			},
			wantPolicies: [][]string{
				{"global-viewer", "namespace:view", "*"},
			},
		},
		{
			name: "add cluster role with multiple actions",
			clusterRole: &authzv1alpha1.ClusterAuthzRole{
				ObjectMeta: metav1.ObjectMeta{Name: "global-admin"},
				Spec: authzv1alpha1.ClusterAuthzRoleSpec{
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
			clusterRole: &authzv1alpha1.ClusterAuthzRole{
				ObjectMeta: metav1.ObjectMeta{Name: "admin"},
				Spec: authzv1alpha1.ClusterAuthzRoleSpec{
					Actions: []string{"*"},
				},
			},
			wantPolicies: [][]string{
				{"admin", "*", "*"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, enforcer := setupTestHandler(t, CRDTypeClusterAuthzRole)

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
	handler, enforcer := setupTestHandler(t, CRDTypeClusterAuthzRole)

	clusterRole := &authzv1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "global-viewer"},
		Spec: authzv1alpha1.ClusterAuthzRoleSpec{
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
		name         string
		binding      *authzv1alpha1.AuthzRoleBinding
		wantPolicies [][]string // Expected policies: [subject, resource, role, namespace, effect, context, binding_name]
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
					RoleMappings: []authzv1alpha1.RoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{
							Kind: CRDTypeAuthzRole,
							Name: "editor",
						},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:developers", "ns/acme", "editor", "acme", "allow", "{}", "dev-binding"},
			},
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
					RoleMappings: []authzv1alpha1.RoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{
							Kind: CRDTypeAuthzRole,
							Name: "viewer",
						},
						Scope: authzv1alpha1.TargetScope{
							Project: "my-project",
						},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:project-team", "ns/acme/project/my-project", "viewer", "acme", "allow", "{}", "project-binding"},
			},
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
					RoleMappings: []authzv1alpha1.RoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{
							Kind: CRDTypeAuthzRole,
							Name: "deployer",
						},
						Scope: authzv1alpha1.TargetScope{
							Project:   "my-project",
							Component: "my-component",
						},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"sub:user-123", "ns/acme/project/my-project/component/my-component", "deployer", "acme", "allow", "{}", "component-binding"},
			},
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
					RoleMappings: []authzv1alpha1.RoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{
							Kind: CRDTypeAuthzRole,
							Name: "editor",
						},
						Scope: authzv1alpha1.TargetScope{
							Project: "secret-project",
						},
					}},
					Effect: authzv1alpha1.EffectDeny,
				},
			},
			wantPolicies: [][]string{
				{"groups:restricted-group", "ns/acme/project/secret-project", "editor", "acme", "deny", "{}", "deny-binding"},
			},
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
					RoleMappings: []authzv1alpha1.RoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{
							Kind: CRDTypeClusterAuthzRole,
							Name: "global-admin",
						},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:admins", "ns/acme", "global-admin", "*", "allow", "{}", "cluster-role-binding"},
			},
		},
		{
			name: "add binding with multiple role mappings",
			binding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "multi-binding", Namespace: "acme"},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "developers",
					},
					RoleMappings: []authzv1alpha1.RoleMapping{
						{
							RoleRef: authzv1alpha1.RoleRef{
								Kind: CRDTypeAuthzRole,
								Name: "editor",
							},
							Scope: authzv1alpha1.TargetScope{
								Project: "project-a",
							},
						},
						{
							RoleRef: authzv1alpha1.RoleRef{
								Kind: CRDTypeAuthzRole,
								Name: "viewer",
							},
							Scope: authzv1alpha1.TargetScope{
								Project: "project-b",
							},
						},
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:developers", "ns/acme/project/project-a", "editor", "acme", "allow", "{}", "multi-binding"},
				{"groups:developers", "ns/acme/project/project-b", "viewer", "acme", "allow", "{}", "multi-binding"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, enforcer := setupTestHandler(t, CRDTypeAuthzRoleBinding)

			err := handler.handleAddBinding(tt.binding)
			if err != nil {
				t.Fatalf("handleAddBinding() error = %v", err)
			}

			for _, wantPolicy := range tt.wantPolicies {
				hasPolicy, err := enforcer.HasPolicy(wantPolicy[0], wantPolicy[1], wantPolicy[2], wantPolicy[3], wantPolicy[4], wantPolicy[5], wantPolicy[6])
				if err != nil {
					t.Fatalf("HasPolicy() error = %v", err)
				}
				if !hasPolicy {
					policies, _ := enforcer.GetPolicy()
					t.Errorf("expected policy %v not found. All policies: %v", wantPolicy, policies)
				}
			}

			// Verify total policy count matches expected
			policies, err := enforcer.GetPolicy()
			if err != nil {
				t.Fatalf("GetPolicy() error = %v", err)
			}
			if len(policies) != len(tt.wantPolicies) {
				t.Errorf("expected %d policies, got %d. All policies: %v", len(tt.wantPolicies), len(policies), policies)
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
					RoleMappings: []authzv1alpha1.RoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{
							Kind: CRDTypeAuthzRole,
							Name: "editor",
						},
					}},
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
					RoleMappings: []authzv1alpha1.RoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{
							Kind: CRDTypeAuthzRole,
							Name: "editor",
						},
					}},
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
			RoleMappings: []authzv1alpha1.RoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{
					Kind: CRDTypeAuthzRole,
					Name: "editor",
				},
			}},
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
		name         string
		binding      *authzv1alpha1.ClusterAuthzRoleBinding
		wantPolicies [][]string // Expected policies: [subject, "*", role, "*", effect, context, binding_name]
	}{
		{
			name: "add cluster binding with allow effect",
			binding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "global-admin-binding"},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "platform-admins",
					},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{
							Kind: CRDTypeClusterAuthzRole,
							Name: "admin",
						},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:platform-admins", "*", "admin", "*", "allow", "{}", "global-admin-binding"},
			},
		},
		{
			name: "add cluster binding with deny effect",
			binding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "global-deny-binding"},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "blocked-group",
					},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{
							Kind: CRDTypeClusterAuthzRole,
							Name: "all-access",
						},
					}},
					Effect: authzv1alpha1.EffectDeny,
				},
			},
			wantPolicies: [][]string{
				{"groups:blocked-group", "*", "all-access", "*", "deny", "{}", "global-deny-binding"},
			},
		},
		{
			name: "add cluster binding with multiple role mappings",
			binding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "multi-cluster-binding"},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "platform-admins",
					},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{
						{
							RoleRef: authzv1alpha1.RoleRef{
								Kind: CRDTypeClusterAuthzRole,
								Name: "super-admin",
							},
						},
						{
							RoleRef: authzv1alpha1.RoleRef{
								Kind: CRDTypeClusterAuthzRole,
								Name: "global-viewer",
							},
						},
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:platform-admins", "*", "super-admin", "*", "allow", "{}", "multi-cluster-binding"},
				{"groups:platform-admins", "*", "global-viewer", "*", "allow", "{}", "multi-cluster-binding"},
			},
		},
		{
			name: "add cluster binding scoped to namespace",
			binding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "ns-scoped-binding"},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "acme-admins",
					},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{
							Kind: CRDTypeClusterAuthzRole,
							Name: "ns-admin",
						},
						Scope: authzv1alpha1.ClusterTargetScope{Namespace: "acme"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:acme-admins", "ns/acme", "ns-admin", "*", "allow", "{}", "ns-scoped-binding"},
			},
		},
		{
			name: "add cluster binding scoped to namespace and project",
			binding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "proj-scoped-binding"},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "proj-team",
					},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{
							Kind: CRDTypeClusterAuthzRole,
							Name: "proj-editor",
						},
						Scope: authzv1alpha1.ClusterTargetScope{Namespace: "acme", Project: "p1"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:proj-team", "ns/acme/project/p1", "proj-editor", "*", "allow", "{}", "proj-scoped-binding"},
			},
		},
		{
			name: "add cluster binding scoped to full hierarchy",
			binding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "full-scoped-binding"},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "comp-team",
					},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{
							Kind: CRDTypeClusterAuthzRole,
							Name: "comp-viewer",
						},
						Scope: authzv1alpha1.ClusterTargetScope{Namespace: "acme", Project: "p1", Component: "c1"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:comp-team", "ns/acme/project/p1/component/c1", "comp-viewer", "*", "allow", "{}", "full-scoped-binding"},
			},
		},
		{
			name: "add cluster binding with mixed scoped and unscoped mappings",
			binding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "mixed-scope-binding"},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{
						Claim: "groups",
						Value: "mixed-team",
					},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{
						{
							RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "global-viewer"},
						},
						{
							RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "ns-editor"},
							Scope:   authzv1alpha1.ClusterTargetScope{Namespace: "acme"},
						},
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:mixed-team", "*", "global-viewer", "*", "allow", "{}", "mixed-scope-binding"},
				{"groups:mixed-team", "ns/acme", "ns-editor", "*", "allow", "{}", "mixed-scope-binding"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, enforcer := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)

			err := handler.handleAddClusterBinding(tt.binding)
			if err != nil {
				t.Fatalf("handleAddClusterBinding() error = %v", err)
			}

			for _, wantPolicy := range tt.wantPolicies {
				hasPolicy, err := enforcer.HasPolicy(wantPolicy[0], wantPolicy[1], wantPolicy[2], wantPolicy[3], wantPolicy[4], wantPolicy[5], wantPolicy[6])
				if err != nil {
					t.Fatalf("HasPolicy() error = %v", err)
				}
				if !hasPolicy {
					policies, _ := enforcer.GetPolicy()
					t.Errorf("expected policy %v not found. All policies: %v", wantPolicy, policies)
				}
			}

			// Verify total policy count matches expected
			policies, err := enforcer.GetPolicy()
			if err != nil {
				t.Fatalf("GetPolicy() error = %v", err)
			}
			if len(policies) != len(tt.wantPolicies) {
				t.Errorf("expected %d policies, got %d. All policies: %v", len(tt.wantPolicies), len(policies), policies)
			}
		})
	}
}

func TestAuthzInformerHandler_HandleAddClusterBinding_Idempotent(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)

	binding := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "global-admin-binding"},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{
				Claim: "groups",
				Value: "platform-admins",
			},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{
					Kind: CRDTypeClusterAuthzRole,
					Name: "admin",
				},
			}},
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
	handler, enforcer := setupTestHandler(t, CRDTypeClusterAuthzRole)

	_, err := enforcer.AddGroupingPolicies([][]string{
		{"global-editor", "namespace:view", "*"},
		{"global-editor", "namespace:create", "*"},
	})
	if err != nil {
		t.Fatalf("AddGroupingPolicies() error = %v", err)
	}

	oldRole := &authzv1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "global-editor",
			Generation: 1,
		},
		Spec: authzv1alpha1.ClusterAuthzRoleSpec{
			Actions: []string{"namespace:view", "namespace:create"},
		},
	}

	newRole := &authzv1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "global-editor",
			Generation: 2,
		},
		Spec: authzv1alpha1.ClusterAuthzRoleSpec{
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

func TestAuthzInformerHandler_HandleUpdateBinding(t *testing.T) {
	tests := []struct {
		name            string
		seedPolicies    [][]string
		oldBinding      *authzv1alpha1.AuthzRoleBinding
		newBinding      *authzv1alpha1.AuthzRoleBinding
		wantPolicies    [][]string
		wantPolicyCount int
	}{
		{
			name: "change role within single mapping",
			seedPolicies: [][]string{
				{"groups:developers", "ns/acme/project/crm", "editor", "acme", "allow", "{}", "dev-binding"},
			},
			oldBinding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: "acme", Generation: 1},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "developers"},
					RoleMappings: []authzv1alpha1.RoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeAuthzRole, Name: "editor"},
						Scope:   authzv1alpha1.TargetScope{Project: "crm"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			newBinding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: "acme", Generation: 2},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "developers"},
					RoleMappings: []authzv1alpha1.RoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeAuthzRole, Name: "viewer"},
						Scope:   authzv1alpha1.TargetScope{Project: "crm"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:developers", "ns/acme/project/crm", "viewer", "acme", "allow", "{}", "dev-binding"},
			},
			wantPolicyCount: 1,
		},
		{
			name: "expand from single to multiple mappings",
			seedPolicies: [][]string{
				{"groups:developers", "ns/acme/project/crm", "editor", "acme", "allow", "{}", "dev-binding"},
			},
			oldBinding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: "acme", Generation: 1},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "developers"},
					RoleMappings: []authzv1alpha1.RoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeAuthzRole, Name: "editor"},
						Scope:   authzv1alpha1.TargetScope{Project: "crm"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			newBinding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: "acme", Generation: 2},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "developers"},
					RoleMappings: []authzv1alpha1.RoleMapping{
						{
							RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeAuthzRole, Name: "editor"},
							Scope:   authzv1alpha1.TargetScope{Project: "crm"},
						},
						{
							RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "viewer"},
						},
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:developers", "ns/acme/project/crm", "editor", "acme", "allow", "{}", "dev-binding"},
				{"groups:developers", "ns/acme", "viewer", "*", "allow", "{}", "dev-binding"},
			},
			wantPolicyCount: 2,
		},
		{
			name: "shrink from multiple to single mapping",
			seedPolicies: [][]string{
				{"groups:developers", "ns/acme/project/crm", "editor", "acme", "allow", "{}", "dev-binding"},
				{"groups:developers", "ns/acme", "viewer", "*", "allow", "{}", "dev-binding"},
			},
			oldBinding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: "acme", Generation: 1},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "developers"},
					RoleMappings: []authzv1alpha1.RoleMapping{
						{
							RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeAuthzRole, Name: "editor"},
							Scope:   authzv1alpha1.TargetScope{Project: "crm"},
						},
						{
							RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "viewer"},
						},
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			newBinding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: "acme", Generation: 2},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "developers"},
					RoleMappings: []authzv1alpha1.RoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeAuthzRole, Name: "editor"},
						Scope:   authzv1alpha1.TargetScope{Project: "crm"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:developers", "ns/acme/project/crm", "editor", "acme", "allow", "{}", "dev-binding"},
			},
			wantPolicyCount: 1,
		},
		{
			name: "change effect from allow to deny",
			seedPolicies: [][]string{
				{"groups:developers", "ns/acme/project/crm", "editor", "acme", "allow", "{}", "dev-binding"},
			},
			oldBinding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: "acme", Generation: 1},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "developers"},
					RoleMappings: []authzv1alpha1.RoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeAuthzRole, Name: "editor"},
						Scope:   authzv1alpha1.TargetScope{Project: "crm"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			newBinding: &authzv1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: "acme", Generation: 2},
				Spec: authzv1alpha1.AuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "developers"},
					RoleMappings: []authzv1alpha1.RoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeAuthzRole, Name: "editor"},
						Scope:   authzv1alpha1.TargetScope{Project: "crm"},
					}},
					Effect: authzv1alpha1.EffectDeny,
				},
			},
			wantPolicies: [][]string{
				{"groups:developers", "ns/acme/project/crm", "editor", "acme", "deny", "{}", "dev-binding"},
			},
			wantPolicyCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, enforcer := setupTestHandler(t, CRDTypeAuthzRoleBinding)

			// Seed existing policies
			if len(tt.seedPolicies) > 0 {
				if _, err := enforcer.AddPolicies(tt.seedPolicies); err != nil {
					t.Fatalf("AddPolicies() error = %v", err)
				}
			}

			if err := handler.handleUpdateBinding(tt.oldBinding, tt.newBinding); err != nil {
				t.Fatalf("handleUpdateBinding() error = %v", err)
			}

			// Verify expected policies exist
			for _, wantPolicy := range tt.wantPolicies {
				hasPolicy, err := enforcer.HasPolicy(wantPolicy[0], wantPolicy[1], wantPolicy[2], wantPolicy[3], wantPolicy[4], wantPolicy[5], wantPolicy[6])
				if err != nil {
					t.Fatalf("HasPolicy() error = %v", err)
				}
				if !hasPolicy {
					policies, _ := enforcer.GetPolicy()
					t.Errorf("expected policy %v not found. All policies: %v", wantPolicy, policies)
				}
			}

			// Verify total count
			policies, err := enforcer.GetPolicy()
			if err != nil {
				t.Fatalf("GetPolicy() error = %v", err)
			}
			if len(policies) != tt.wantPolicyCount {
				t.Errorf("expected %d policies, got %d. All policies: %v", tt.wantPolicyCount, len(policies), policies)
			}
		})
	}
}

func TestAuthzInformerHandler_HandleUpdateBinding_NoGenerationChange(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeAuthzRoleBinding)

	_, err := enforcer.AddPolicy("groups:developers", "ns/acme/project/crm", "editor", "acme", "allow", "{}", "dev-binding")
	if err != nil {
		t.Fatalf("AddPolicy() error = %v", err)
	}

	binding := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: "acme", Generation: 1},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "developers"},
			RoleMappings: []authzv1alpha1.RoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeAuthzRole, Name: "editor"},
				Scope:   authzv1alpha1.TargetScope{Project: "crm"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}

	// Same generation — should be a no-op
	newBinding := binding.DeepCopy()
	newBinding.Spec.RoleMappings[0].RoleRef.Name = "viewer"

	if err := handler.handleUpdateBinding(binding, newBinding); err != nil {
		t.Fatalf("handleUpdateBinding() error = %v", err)
	}

	// Original policy should remain unchanged
	hasOld, _ := enforcer.HasPolicy("groups:developers", "ns/acme/project/crm", "editor", "acme", "allow", "{}", "dev-binding")
	hasNew, _ := enforcer.HasPolicy("groups:developers", "ns/acme/project/crm", "viewer", "acme", "allow", "{}", "dev-binding")
	if !hasOld {
		t.Error("original policy should still exist (update skipped)")
	}
	if hasNew {
		t.Error("new policy should not be added (update skipped)")
	}
}

func TestAuthzInformerHandler_HandleUpdateClusterBinding(t *testing.T) {
	tests := []struct {
		name            string
		seedPolicies    [][]string
		oldBinding      *authzv1alpha1.ClusterAuthzRoleBinding
		newBinding      *authzv1alpha1.ClusterAuthzRoleBinding
		wantPolicies    [][]string
		wantPolicyCount int
	}{
		{
			name: "change role within single mapping",
			seedPolicies: [][]string{
				{"groups:admins", "*", "super-admin", "*", "allow", "{}", "admin-binding"},
			},
			oldBinding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "admin-binding", Generation: 1},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "super-admin"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			newBinding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "admin-binding", Generation: 2},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "global-viewer"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:admins", "*", "global-viewer", "*", "allow", "{}", "admin-binding"},
			},
			wantPolicyCount: 1,
		},
		{
			name: "expand from single to multiple mappings",
			seedPolicies: [][]string{
				{"groups:admins", "*", "super-admin", "*", "allow", "{}", "admin-binding"},
			},
			oldBinding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "admin-binding", Generation: 1},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "super-admin"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			newBinding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "admin-binding", Generation: 2},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{
						{RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "super-admin"}},
						{RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "audit-viewer"}},
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:admins", "*", "super-admin", "*", "allow", "{}", "admin-binding"},
				{"groups:admins", "*", "audit-viewer", "*", "allow", "{}", "admin-binding"},
			},
			wantPolicyCount: 2,
		},
		{
			name: "shrink from multiple to single mapping",
			seedPolicies: [][]string{
				{"groups:admins", "*", "super-admin", "*", "allow", "{}", "admin-binding"},
				{"groups:admins", "*", "audit-viewer", "*", "allow", "{}", "admin-binding"},
			},
			oldBinding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "admin-binding", Generation: 1},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{
						{RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "super-admin"}},
						{RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "audit-viewer"}},
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			newBinding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "admin-binding", Generation: 2},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "super-admin"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:admins", "*", "super-admin", "*", "allow", "{}", "admin-binding"},
			},
			wantPolicyCount: 1,
		},
		{
			name: "update unscoped to namespace-scoped",
			seedPolicies: [][]string{
				{"groups:admins", "*", "super-admin", "*", "allow", "{}", "scope-change-binding"},
			},
			oldBinding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "scope-change-binding", Generation: 1},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "super-admin"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			newBinding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "scope-change-binding", Generation: 2},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "super-admin"},
						Scope:   authzv1alpha1.ClusterTargetScope{Namespace: "acme"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:admins", "ns/acme", "super-admin", "*", "allow", "{}", "scope-change-binding"},
			},
			wantPolicyCount: 1,
		},
		{
			name: "add scoped mapping to existing binding",
			seedPolicies: [][]string{
				{"groups:admins", "*", "super-admin", "*", "allow", "{}", "add-scope-binding"},
			},
			oldBinding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "add-scope-binding", Generation: 1},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "super-admin"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			newBinding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "add-scope-binding", Generation: 2},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{
						{RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "super-admin"}},
						{
							RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "ns-viewer"},
							Scope:   authzv1alpha1.ClusterTargetScope{Namespace: "acme"},
						},
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:admins", "*", "super-admin", "*", "allow", "{}", "add-scope-binding"},
				{"groups:admins", "ns/acme", "ns-viewer", "*", "allow", "{}", "add-scope-binding"},
			},
			wantPolicyCount: 2,
		},
		{
			name: "remove scoped mapping from binding",
			seedPolicies: [][]string{
				{"groups:admins", "*", "super-admin", "*", "allow", "{}", "rm-scope-binding"},
				{"groups:admins", "ns/acme", "ns-viewer", "*", "allow", "{}", "rm-scope-binding"},
			},
			oldBinding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "rm-scope-binding", Generation: 1},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{
						{RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "super-admin"}},
						{
							RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "ns-viewer"},
							Scope:   authzv1alpha1.ClusterTargetScope{Namespace: "acme"},
						},
					},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			newBinding: &authzv1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "rm-scope-binding", Generation: 2},
				Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
					RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
						RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "super-admin"},
					}},
					Effect: authzv1alpha1.EffectAllow,
				},
			},
			wantPolicies: [][]string{
				{"groups:admins", "*", "super-admin", "*", "allow", "{}", "rm-scope-binding"},
			},
			wantPolicyCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, enforcer := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)

			// Seed existing policies
			if len(tt.seedPolicies) > 0 {
				if _, err := enforcer.AddPolicies(tt.seedPolicies); err != nil {
					t.Fatalf("AddPolicies() error = %v", err)
				}
			}

			if err := handler.handleUpdateClusterBinding(tt.oldBinding, tt.newBinding); err != nil {
				t.Fatalf("handleUpdateClusterBinding() error = %v", err)
			}

			// Verify expected policies exist
			for _, wantPolicy := range tt.wantPolicies {
				hasPolicy, err := enforcer.HasPolicy(wantPolicy[0], wantPolicy[1], wantPolicy[2], wantPolicy[3], wantPolicy[4], wantPolicy[5], wantPolicy[6])
				if err != nil {
					t.Fatalf("HasPolicy() error = %v", err)
				}
				if !hasPolicy {
					policies, _ := enforcer.GetPolicy()
					t.Errorf("expected policy %v not found. All policies: %v", wantPolicy, policies)
				}
			}

			// Verify total count
			policies, err := enforcer.GetPolicy()
			if err != nil {
				t.Fatalf("GetPolicy() error = %v", err)
			}
			if len(policies) != tt.wantPolicyCount {
				t.Errorf("expected %d policies, got %d. All policies: %v", tt.wantPolicyCount, len(policies), policies)
			}
		})
	}
}

func TestAuthzInformerHandler_HandleUpdateClusterBinding_NoGenerationChange(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)

	_, err := enforcer.AddPolicy("groups:admins", "*", "super-admin", "*", "allow", "{}", "admin-binding")
	if err != nil {
		t.Fatalf("AddPolicy() error = %v", err)
	}

	binding := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-binding", Generation: 1},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "super-admin"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}

	// Same generation — should be a no-op
	newBinding := binding.DeepCopy()
	newBinding.Spec.RoleMappings[0].RoleRef.Name = "global-viewer"

	if err := handler.handleUpdateClusterBinding(binding, newBinding); err != nil {
		t.Fatalf("handleUpdateClusterBinding() error = %v", err)
	}

	// Original policy should remain unchanged
	hasOld, _ := enforcer.HasPolicy("groups:admins", "*", "super-admin", "*", "allow", "{}", "admin-binding")
	hasNew, _ := enforcer.HasPolicy("groups:admins", "*", "global-viewer", "*", "allow", "{}", "admin-binding")
	if !hasOld {
		t.Error("original policy should still exist (update skipped)")
	}
	if hasNew {
		t.Error("new policy should not be added (update skipped)")
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
	handler, enforcer := setupTestHandler(t, CRDTypeClusterAuthzRole)

	_, err := enforcer.AddGroupingPolicies([][]string{
		{"global-admin", "namespace:view", "*"},
		{"global-admin", "namespace:create", "*"},
	})
	if err != nil {
		t.Fatalf("AddGroupingPolicies() error = %v", err)
	}

	clusterRole := &authzv1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "global-admin"},
		Spec: authzv1alpha1.ClusterAuthzRoleSpec{
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
			RoleMappings: []authzv1alpha1.RoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{
					Kind: CRDTypeAuthzRole,
					Name: "editor",
				},
			}},
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

func TestAuthzInformerHandler_HandleDeleteBinding_MultipleRoleMappings(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeAuthzRoleBinding)

	// Pre-add 2 policies matching 2 role mappings
	_, err := enforcer.AddPolicies([][]string{
		{"groups:developers", "ns/acme/project/project-a", "editor", "acme", "allow", "{}", "multi-binding"},
		{"groups:developers", "ns/acme/project/project-b", "viewer", "acme", "allow", "{}", "multi-binding"},
	})
	if err != nil {
		t.Fatalf("AddPolicies() error = %v", err)
	}

	binding := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "multi-binding", Namespace: "acme"},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{
				Claim: "groups",
				Value: "developers",
			},
			RoleMappings: []authzv1alpha1.RoleMapping{
				{
					RoleRef: authzv1alpha1.RoleRef{
						Kind: CRDTypeAuthzRole,
						Name: "editor",
					},
					Scope: authzv1alpha1.TargetScope{
						Project: "project-a",
					},
				},
				{
					RoleRef: authzv1alpha1.RoleRef{
						Kind: CRDTypeAuthzRole,
						Name: "viewer",
					},
					Scope: authzv1alpha1.TargetScope{
						Project: "project-b",
					},
				},
			},
			Effect: authzv1alpha1.EffectAllow,
		},
	}

	if err := handler.handleDeleteBinding(binding); err != nil {
		t.Fatalf("handleDeleteBinding() error = %v", err)
	}

	// Verify both policies are removed
	policies, err := enforcer.GetPolicy()
	if err != nil {
		t.Fatalf("GetPolicy() error = %v", err)
	}
	if len(policies) != 0 {
		t.Errorf("expected 0 policies after delete, got %d. All policies: %v", len(policies), policies)
	}
}

func TestAuthzInformerHandler_HandleDeleteClusterBinding(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)

	// Setup: directly add policy to Casbin
	_, err := enforcer.AddPolicy("groups:platform-admins", "*", "admin", "*", "allow", "{}", "global-admin-binding")
	if err != nil {
		t.Fatalf("AddPolicy() error = %v", err)
	}

	binding := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "global-admin-binding"},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{
				Claim: "groups",
				Value: "platform-admins",
			},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{
					Kind: CRDTypeClusterAuthzRole,
					Name: "admin",
				},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}

	if err := handler.handleDeleteClusterBinding(binding); err != nil {
		t.Fatalf("handleDeleteClusterBinding() error = %v", err)
	}

	hasPolicy, _ := enforcer.HasPolicy("groups:platform-admins", "*", "admin", "*", "allow", "{}", "global-admin-binding")
	if hasPolicy {
		t.Error("policy should be removed after delete")
	}
}

func TestAuthzInformerHandler_HandleDeleteClusterBinding_MultipleRoleMappings(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)

	// Pre-add 2 policies matching 2 cluster role mappings
	_, err := enforcer.AddPolicies([][]string{
		{"groups:platform-admins", "*", "super-admin", "*", "allow", "{}", "multi-cluster-binding"},
		{"groups:platform-admins", "*", "global-viewer", "*", "allow", "{}", "multi-cluster-binding"},
	})
	if err != nil {
		t.Fatalf("AddPolicies() error = %v", err)
	}

	binding := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "multi-cluster-binding"},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{
				Claim: "groups",
				Value: "platform-admins",
			},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{
				{
					RoleRef: authzv1alpha1.RoleRef{
						Kind: CRDTypeClusterAuthzRole,
						Name: "super-admin",
					},
				},
				{
					RoleRef: authzv1alpha1.RoleRef{
						Kind: CRDTypeClusterAuthzRole,
						Name: "global-viewer",
					},
				},
			},
			Effect: authzv1alpha1.EffectAllow,
		},
	}

	if err := handler.handleDeleteClusterBinding(binding); err != nil {
		t.Fatalf("handleDeleteClusterBinding() error = %v", err)
	}

	// Verify both policies are removed
	policies, err := enforcer.GetPolicy()
	if err != nil {
		t.Fatalf("GetPolicy() error = %v", err)
	}
	if len(policies) != 0 {
		t.Errorf("expected 0 policies after delete, got %d. All policies: %v", len(policies), policies)
	}
}

func TestAuthzInformerHandler_HandleDeleteClusterBinding_Scoped(t *testing.T) {
	handler, enforcer := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)

	// Pre-add scoped policies
	_, err := enforcer.AddPolicies([][]string{
		{"groups:admins", "*", "global-admin", "*", "allow", "{}", "scoped-del-binding"},
		{"groups:admins", "ns/acme", "ns-viewer", "*", "allow", "{}", "scoped-del-binding"},
	})
	if err != nil {
		t.Fatalf("AddPolicies() error = %v", err)
	}

	binding := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "scoped-del-binding"},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{
				Claim: "groups",
				Value: "admins",
			},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{
				{
					RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "global-admin"},
				},
				{
					RoleRef: authzv1alpha1.RoleRef{Kind: CRDTypeClusterAuthzRole, Name: "ns-viewer"},
					Scope:   authzv1alpha1.ClusterTargetScope{Namespace: "acme"},
				},
			},
			Effect: authzv1alpha1.EffectAllow,
		},
	}

	if err := handler.handleDeleteClusterBinding(binding); err != nil {
		t.Fatalf("handleDeleteClusterBinding() error = %v", err)
	}

	policies, err := enforcer.GetPolicy()
	if err != nil {
		t.Fatalf("GetPolicy() error = %v", err)
	}
	if len(policies) != 0 {
		t.Errorf("expected 0 policies after delete, got %d. All policies: %v", len(policies), policies)
	}
}

// --- OnAdd/OnUpdate/OnDelete wrappers ---

func TestAuthzInformerHandler_OnAdd_Role(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRole)
	role := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "on-add-role", Namespace: "acme"},
		Spec:       authzv1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
	}
	h.OnAdd(role, false) // should not panic
}

func TestAuthzInformerHandler_OnUpdate_Role(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRole)
	old := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "on-upd-role", Namespace: "acme", Generation: 1},
		Spec:       authzv1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
	}
	newR := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "on-upd-role", Namespace: "acme", Generation: 2},
		Spec:       authzv1alpha1.AuthzRoleSpec{Actions: []string{"component:create"}},
	}
	// Seed the old policy so the update handler can remove it
	_, _ = h.enforcer.AddGroupingPolicy("on-upd-role", "component:view", "acme")
	h.OnUpdate(old, newR)
	// Verify the OnUpdate wrapper delegated correctly: old action removed, new action added
	hasOld, _ := h.enforcer.HasGroupingPolicy("on-upd-role", "component:view", "acme")
	require.False(t, hasOld, "old action component:view should be removed after OnUpdate")
	hasNew, _ := h.enforcer.HasGroupingPolicy("on-upd-role", "component:create", "acme")
	require.True(t, hasNew, "new action component:create should be added after OnUpdate")
}

func TestAuthzInformerHandler_OnDelete_Role(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRole)
	role := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "on-del-role", Namespace: "acme"},
		Spec:       authzv1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
	}
	h.OnDelete(role) // should not panic
}

func TestAuthzInformerHandler_OnDelete_WithTombstone(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRole)
	role := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "tombstone-role", Namespace: "acme"},
		Spec:       authzv1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
	}
	// Wrap in a DeletedFinalStateUnknown tombstone
	tombstone := cache.DeletedFinalStateUnknown{
		Key: "acme/tombstone-role",
		Obj: role,
	}
	h.OnDelete(tombstone) // should not panic
}

// --- handleAdd dispatcher ---

func TestAuthzInformerHandler_HandleAdd_UnknownCRDType(t *testing.T) {
	h, _ := setupTestHandler(t, "UnknownType")
	role := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "acme"},
		Spec:       authzv1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
	}
	err := h.handleAdd(role)
	require.NoError(t, err, "handleAdd with unknown crdType should return nil")
}

func TestAuthzInformerHandler_HandleAdd_ClusterRole(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRole)
	cr := &authzv1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "global-admin"},
		Spec:       authzv1alpha1.ClusterAuthzRoleSpec{Actions: []string{"*"}},
	}
	require.NoError(t, h.handleAdd(cr), "handleAdd ClusterRole error")
	// Verify the grouping policy was added: [roleName, action, "*"]
	hasGP, _ := h.enforcer.HasGroupingPolicy("global-admin", "*", "*")
	require.True(t, hasGP, "expected grouping policy [global-admin, *, *] to exist after handleAdd")
}

func TestAuthzInformerHandler_HandleAdd_Binding(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	binding := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "b1", Namespace: "acme"},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       authzv1alpha1.EffectAllow,
		},
	}
	require.NoError(t, h.handleAdd(binding), "handleAdd Binding error")
	// Verify policy tuple: [subject, resource, roleName, roleNamespace, effect, ctx, bindingRef]
	hasP, _ := h.enforcer.HasPolicy("groups:devs", "ns/acme", "viewer", "acme", "allow", "{}", "b1")
	require.True(t, hasP, "expected policy [groups:devs, ns/acme, viewer, acme, allow, {}, b1] after handleAdd")
}

func TestAuthzInformerHandler_HandleAdd_ClusterBinding(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	binding := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "cb1"},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "global-admin"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	require.NoError(t, h.handleAdd(binding), "handleAdd ClusterBinding error")
	// Verify policy tuple: cluster bindings use resource "*" (empty scope) and roleNamespace "*"
	hasP, _ := h.enforcer.HasPolicy("groups:admins", "*", "global-admin", "*", "allow", "{}", "cb1")
	require.True(t, hasP, "expected policy [groups:admins, *, global-admin, *, allow, {}, cb1] after handleAdd")
}

// --- handleUpdate dispatcher ---

func TestAuthzInformerHandler_HandleUpdate_UnknownCRDType(t *testing.T) {
	h, _ := setupTestHandler(t, "UnknownType")
	old := &authzv1alpha1.AuthzRole{ObjectMeta: metav1.ObjectMeta{Name: "r"}}
	newR := &authzv1alpha1.AuthzRole{ObjectMeta: metav1.ObjectMeta{Name: "r"}}
	err := h.handleUpdate(old, newR)
	require.NoError(t, err, "handleUpdate with unknown crdType should return nil")
}

func TestAuthzInformerHandler_HandleUpdate_ClusterRole(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRole)
	old := &authzv1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "cr", Generation: 1},
		Spec:       authzv1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:view"}},
	}
	newR := &authzv1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "cr", Generation: 2},
		Spec:       authzv1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:create"}},
	}
	_, _ = h.enforcer.AddGroupingPolicy("cr", "component:view", "*")
	require.NoError(t, h.handleUpdate(old, newR), "handleUpdate ClusterRole error")
	// Old action must be removed, new action must be added
	hasOld, _ := h.enforcer.HasGroupingPolicy("cr", "component:view", "*")
	require.False(t, hasOld, "old grouping policy [cr, component:view, *] should be removed after update")
	hasNew, _ := h.enforcer.HasGroupingPolicy("cr", "component:create", "*")
	require.True(t, hasNew, "new grouping policy [cr, component:create, *] should exist after update")
}

func TestAuthzInformerHandler_HandleUpdate_Binding(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	old := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "acme", Generation: 1},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       authzv1alpha1.EffectAllow,
		},
	}
	newB := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "acme", Generation: 2},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "editor"}}},
			Effect:       authzv1alpha1.EffectAllow,
		},
	}
	// Seed the old policy
	_, _ = h.enforcer.AddPoliciesEx([][]string{{"groups:devs", "ns/acme", "viewer", "acme", "allow", "{}", "b"}})
	require.NoError(t, h.handleUpdate(old, newB), "handleUpdate Binding error")
	// Old policy (viewer) must be removed, new policy (editor) must be added
	hasOld, _ := h.enforcer.HasPolicy("groups:devs", "ns/acme", "viewer", "acme", "allow", "{}", "b")
	require.False(t, hasOld, "old policy (viewer) should be removed after binding update")
	hasNew, _ := h.enforcer.HasPolicy("groups:devs", "ns/acme", "editor", "acme", "allow", "{}", "b")
	require.True(t, hasNew, "new policy (editor) should exist after binding update")
}

func TestAuthzInformerHandler_HandleUpdate_ClusterBinding(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	old := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "cb", Generation: 1},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "viewer"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	newB := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "cb", Generation: 2},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "editor"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	// Seed the old policy
	_, _ = h.enforcer.AddPoliciesEx([][]string{{"groups:admins", "*", "viewer", "*", "allow", "{}", "cb"}})
	require.NoError(t, h.handleUpdate(old, newB), "handleUpdate ClusterBinding error")
	// Old policy (viewer) must be removed, new policy (editor) must be added
	hasOld, _ := h.enforcer.HasPolicy("groups:admins", "*", "viewer", "*", "allow", "{}", "cb")
	require.False(t, hasOld, "old policy (viewer) should be removed after cluster binding update")
	hasNew, _ := h.enforcer.HasPolicy("groups:admins", "*", "editor", "*", "allow", "{}", "cb")
	require.True(t, hasNew, "new policy (editor) should exist after cluster binding update")
}

// --- handleDelete dispatcher ---

func TestAuthzInformerHandler_HandleDelete_UnknownCRDType(t *testing.T) {
	h, _ := setupTestHandler(t, "UnknownType")
	role := &authzv1alpha1.AuthzRole{ObjectMeta: metav1.ObjectMeta{Name: "r"}}
	err := h.handleDelete(role)
	require.NoError(t, err, "handleDelete with unknown crdType should return nil")
}

func TestAuthzInformerHandler_HandleDelete_ClusterRole(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRole)
	cr := &authzv1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "cr-del"},
		Spec:       authzv1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:view"}},
	}
	// Seed the policy so deletion has something to remove
	_, _ = h.enforcer.AddGroupingPoliciesEx([][]string{{"cr-del", "component:view", "*"}})
	require.NoError(t, h.handleDelete(cr), "handleDelete ClusterRole error")
	// Verify the grouping policy was removed
	hasGP, _ := h.enforcer.HasGroupingPolicy("cr-del", "component:view", "*")
	require.False(t, hasGP, "grouping policy should be removed after handleDelete ClusterRole")
}

func TestAuthzInformerHandler_HandleDelete_Binding(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	binding := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "b-del", Namespace: "acme"},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       authzv1alpha1.EffectAllow,
		},
	}
	// Seed the policy so deletion has something to remove
	_, _ = h.enforcer.AddPoliciesEx([][]string{{"groups:devs", "ns/acme", "viewer", "acme", "allow", "{}", "b-del"}})
	require.NoError(t, h.handleDelete(binding), "handleDelete Binding error")
	// Verify the policy was removed
	hasP, _ := h.enforcer.HasPolicy("groups:devs", "ns/acme", "viewer", "acme", "allow", "{}", "b-del")
	require.False(t, hasP, "policy should be removed after handleDelete Binding")
}

func TestAuthzInformerHandler_HandleDelete_ClusterBinding(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	binding := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "cb-del"},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "global-admin"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	// Seed the policy so deletion has something to remove
	_, _ = h.enforcer.AddPoliciesEx([][]string{{"groups:admins", "*", "global-admin", "*", "allow", "{}", "cb-del"}})
	require.NoError(t, h.handleDelete(binding), "handleDelete ClusterBinding error")
	// Verify the policy was removed
	hasP, _ := h.enforcer.HasPolicy("groups:admins", "*", "global-admin", "*", "allow", "{}", "cb-del")
	require.False(t, hasP, "policy should be removed after handleDelete ClusterBinding")
}

// --- Wrong-type object paths ---

func TestAuthzInformerHandler_HandleAddRole_WrongType(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRole)
	err := h.handleAddRole("not-an-authz-role")
	require.NoError(t, err, "handleAddRole with wrong type should return nil")
}

func TestAuthzInformerHandler_HandleAddClusterRole_WrongType(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRole)
	err := h.handleAddClusterRole("not-a-cluster-role")
	require.NoError(t, err, "handleAddClusterRole with wrong type should return nil")
}

func TestAuthzInformerHandler_HandleAddBinding_WrongType(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	err := h.handleAddBinding("not-a-binding")
	require.NoError(t, err, "handleAddBinding with wrong type should return nil")
}

func TestAuthzInformerHandler_HandleAddBinding_EmptyEffect(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	binding := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "empty-effect", Namespace: "acme"},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       "", // empty effect
		},
	}
	err := h.handleAddBinding(binding)
	require.Error(t, err, "handleAddBinding with empty effect should return error")
}

func TestAuthzInformerHandler_HandleAddClusterBinding_WrongType(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	err := h.handleAddClusterBinding("not-a-cluster-binding")
	require.NoError(t, err, "handleAddClusterBinding with wrong type should return nil")
}

func TestAuthzInformerHandler_HandleAddClusterBinding_EmptyEffect(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	binding := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "empty-effect-cb"},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"},
			}},
			Effect: "", // empty effect
		},
	}
	err := h.handleAddClusterBinding(binding)
	require.Error(t, err, "handleAddClusterBinding with empty effect should return error")
}

// --- UpdateRole edge cases ---

func TestAuthzInformerHandler_HandleUpdateRole_WrongType(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRole)
	err := h.handleUpdateRole("not-a-role", "also-not-a-role")
	require.NoError(t, err, "handleUpdateRole with wrong type should return nil")
}

func TestAuthzInformerHandler_HandleUpdateRole_SameGeneration(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRole)
	role := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "same-gen", Namespace: "acme", Generation: 5},
		Spec:       authzv1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
	}
	// same generation → should skip and return nil
	err := h.handleUpdateRole(role, role)
	require.NoError(t, err, "handleUpdateRole same generation should return nil")
}

// --- UpdateClusterRole edge cases ---

func TestAuthzInformerHandler_HandleUpdateClusterRole_WrongType(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRole)
	err := h.handleUpdateClusterRole("not-a-role", "also-not-a-role")
	require.NoError(t, err, "handleUpdateClusterRole with wrong type should return nil")
}

func TestAuthzInformerHandler_HandleUpdateClusterRole_SameGeneration(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRole)
	role := &authzv1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "same-gen-cr", Generation: 3},
		Spec:       authzv1alpha1.ClusterAuthzRoleSpec{Actions: []string{"*"}},
	}
	err := h.handleUpdateClusterRole(role, role)
	require.NoError(t, err, "handleUpdateClusterRole same generation should return nil")
}

// --- UpdateBinding edge cases ---

func TestAuthzInformerHandler_HandleUpdateBinding_WrongType(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	err := h.handleUpdateBinding("not-a-binding", "also-not-a-binding")
	require.NoError(t, err, "handleUpdateBinding with wrong type should return nil")
}

func TestAuthzInformerHandler_HandleUpdateBinding_SameGeneration(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	binding := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "same-gen-b", Namespace: "acme", Generation: 7},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       authzv1alpha1.EffectAllow,
		},
	}
	err := h.handleUpdateBinding(binding, binding)
	require.NoError(t, err, "handleUpdateBinding same generation should return nil")
}

func TestAuthzInformerHandler_HandleUpdateBinding_EmptyOldEffect(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	old := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "b-empty-old", Namespace: "acme", Generation: 1},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       "", // empty
		},
	}
	newB := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "b-empty-old", Namespace: "acme", Generation: 2},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       authzv1alpha1.EffectAllow,
		},
	}
	err := h.handleUpdateBinding(old, newB)
	require.Error(t, err, "handleUpdateBinding with empty old effect should return error")
}

func TestAuthzInformerHandler_HandleUpdateBinding_EmptyNewEffect(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	old := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "b-empty-new", Namespace: "acme", Generation: 1},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       authzv1alpha1.EffectAllow,
		},
	}
	newB := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "b-empty-new", Namespace: "acme", Generation: 2},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       "", // empty
		},
	}
	err := h.handleUpdateBinding(old, newB)
	require.Error(t, err, "handleUpdateBinding with empty new effect should return error")
}

// --- UpdateClusterBinding edge cases ---

func TestAuthzInformerHandler_HandleUpdateClusterBinding_WrongType(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	err := h.handleUpdateClusterBinding("not-a-binding", "also-not-a-binding")
	require.NoError(t, err, "handleUpdateClusterBinding with wrong type should return nil")
}

func TestAuthzInformerHandler_HandleUpdateClusterBinding_SameGeneration(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	binding := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "same-gen-cb", Generation: 4},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	err := h.handleUpdateClusterBinding(binding, binding)
	require.NoError(t, err, "handleUpdateClusterBinding same generation should return nil")
}

func TestAuthzInformerHandler_HandleUpdateClusterBinding_EmptyOldEffect(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	old := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "cb-empty-old", Generation: 1},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"},
			}},
			Effect: "", // empty
		},
	}
	newB := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "cb-empty-old", Generation: 2},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	err := h.handleUpdateClusterBinding(old, newB)
	require.Error(t, err, "handleUpdateClusterBinding with empty old effect should return error")
}

func TestAuthzInformerHandler_HandleUpdateClusterBinding_EmptyNewEffect(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	old := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "cb-empty-new", Generation: 1},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	newB := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "cb-empty-new", Generation: 2},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"},
			}},
			Effect: "", // empty
		},
	}
	err := h.handleUpdateClusterBinding(old, newB)
	require.Error(t, err, "handleUpdateClusterBinding with empty new effect should return error")
}

// --- DeleteRole edge cases ---

func TestAuthzInformerHandler_HandleDeleteRole_WrongType(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRole)
	err := h.handleDeleteRole("not-a-role")
	require.NoError(t, err, "handleDeleteRole with wrong type should return nil")
}

func TestAuthzInformerHandler_HandleDeleteClusterRole_WrongType(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRole)
	err := h.handleDeleteClusterRole("not-a-cluster-role")
	require.NoError(t, err, "handleDeleteClusterRole with wrong type should return nil")
}

func TestAuthzInformerHandler_HandleDeleteBinding_WrongType(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	err := h.handleDeleteBinding("not-a-binding")
	require.NoError(t, err, "handleDeleteBinding with wrong type should return nil")
}

func TestAuthzInformerHandler_HandleDeleteBinding_EmptyEffect(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	binding := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "del-empty-effect", Namespace: "acme"},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       "", // empty
		},
	}
	err := h.handleDeleteBinding(binding)
	require.Error(t, err, "handleDeleteBinding with empty effect should return error")
}

func TestAuthzInformerHandler_HandleDeleteClusterBinding_WrongType(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	err := h.handleDeleteClusterBinding("not-a-cluster-binding")
	require.NoError(t, err, "handleDeleteClusterBinding with wrong type should return nil")
}

func TestAuthzInformerHandler_HandleDeleteClusterBinding_EmptyEffect(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	binding := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "del-cb-empty-effect"},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"},
			}},
			Effect: "", // empty
		},
	}
	err := h.handleDeleteClusterBinding(binding)
	require.Error(t, err, "handleDeleteClusterBinding with empty effect should return error")
}

// --- OnAdd/OnUpdate/OnDelete error-logging paths ---

// OnAdd logs the error when the handler returns one (e.g. empty effect in binding).
func TestAuthzInformerHandler_OnAdd_ErrorLogged(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	var logs bytes.Buffer
	h.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	badBinding := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "on-add-err", Namespace: "acme"},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       "", // causes handleAddBinding to return an error
		},
	}
	h.OnAdd(badBinding, false)
	require.Contains(t, logs.String(), "Incremental add failed")
	require.Contains(t, logs.String(), "level=ERROR")
}

// OnUpdate logs the error when the handler returns one.
func TestAuthzInformerHandler_OnUpdate_ErrorLogged(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	var logs bytes.Buffer
	h.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	// Old binding with empty effect → handleUpdateBinding errors immediately on old effect
	old := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "on-upd-err", Namespace: "acme", Generation: 1},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       "", // empty → triggers error in handleUpdateBinding
		},
	}
	newB := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "on-upd-err", Namespace: "acme", Generation: 2},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "editor"}}},
			Effect:       authzv1alpha1.EffectAllow,
		},
	}
	h.OnUpdate(old, newB)
	require.Contains(t, logs.String(), "Incremental update failed")
	require.Contains(t, logs.String(), "level=ERROR")
}

// OnDelete logs the error when the handler returns one.
func TestAuthzInformerHandler_OnDelete_ErrorLogged(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	var logs bytes.Buffer
	h.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	badBinding := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "on-del-err", Namespace: "acme"},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       "", // empty → triggers error in handleDeleteBinding
		},
	}
	h.OnDelete(badBinding)
	require.Contains(t, logs.String(), "Incremental delete failed")
	require.Contains(t, logs.String(), "level=WARN")
}

// --- handleAddClusterBinding — formatSubject error (empty claim) ---

func TestAuthzInformerHandler_HandleAddClusterBinding_EmptyClaim(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	binding := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "empty-claim-cb"},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "", Value: "admins"}, // empty claim
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	err := h.handleAddClusterBinding(binding)
	require.Error(t, err, "handleAddClusterBinding with empty claim should return error")
}

// --- handleDeleteBinding / handleDeleteClusterBinding — formatSubject errors ---

func TestAuthzInformerHandler_HandleDeleteBinding_EmptyClaim(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	binding := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "del-empty-claim", Namespace: "acme"},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "", Value: "devs"}, // empty claim
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       authzv1alpha1.EffectAllow,
		},
	}
	err := h.handleDeleteBinding(binding)
	require.Error(t, err, "handleDeleteBinding with empty claim should return error")
}

func TestAuthzInformerHandler_HandleDeleteClusterBinding_EmptyClaim(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	binding := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "del-cb-empty-claim"},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "", Value: "admins"}, // empty claim
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	err := h.handleDeleteClusterBinding(binding)
	require.Error(t, err, "handleDeleteClusterBinding with empty claim should return error")
}

// --- handleUpdateRole / handleUpdateClusterRole — "not removed" and "not added" branches ---

// When an action to be removed was never in the enforcer, RemoveGroupingPolicy
// returns (false, nil) and the handler logs a debug message.
func TestAuthzInformerHandler_HandleUpdateRole_ActionNotRemovedNotInEnforcer(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRole)

	// Old role has "component:view", new role has "component:create".
	// We do NOT pre-seed "component:view" in the enforcer, so removal returns false.
	old := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "update-not-removed", Namespace: "acme", Generation: 1},
		Spec:       authzv1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
	}
	newR := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "update-not-removed", Namespace: "acme", Generation: 2},
		Spec:       authzv1alpha1.AuthzRoleSpec{Actions: []string{"component:create"}},
	}
	require.NoError(t, h.handleUpdateRole(old, newR), "handleUpdateRole should succeed even when policy was not in enforcer")
}

// When an action to be added is already in the enforcer, AddGroupingPolicy
// returns (false, nil) and the handler logs a debug message.
func TestAuthzInformerHandler_HandleUpdateRole_ActionNotAddedAlreadyExists(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRole)

	// Pre-seed "component:create" so it already exists.
	_, _ = h.enforcer.AddGroupingPolicy("update-already-added", "component:create", "acme")
	_, _ = h.enforcer.AddGroupingPolicy("update-already-added", "component:view", "acme")

	old := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "update-already-added", Namespace: "acme", Generation: 1},
		Spec:       authzv1alpha1.AuthzRoleSpec{Actions: []string{"component:view"}},
	}
	newR := &authzv1alpha1.AuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "update-already-added", Namespace: "acme", Generation: 2},
		Spec:       authzv1alpha1.AuthzRoleSpec{Actions: []string{"component:create"}},
	}
	require.NoError(t, h.handleUpdateRole(old, newR), "handleUpdateRole should succeed even when new action already exists")
}

// handleUpdateClusterRole — "Old cluster grouping policy did not exist"
func TestAuthzInformerHandler_HandleUpdateClusterRole_ActionNotRemovedNotInEnforcer(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRole)

	old := &authzv1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "cr-not-removed", Generation: 1},
		Spec:       authzv1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:view"}},
	}
	newR := &authzv1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "cr-not-removed", Generation: 2},
		Spec:       authzv1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:create"}},
	}
	// Do NOT seed "component:view" — RemoveGroupingPolicy will return false.
	require.NoError(t, h.handleUpdateClusterRole(old, newR), "handleUpdateClusterRole should succeed even when policy was not in enforcer")
}

// handleUpdateClusterRole — "New cluster grouping policy already exists"
func TestAuthzInformerHandler_HandleUpdateClusterRole_ActionNotAddedAlreadyExists(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRole)

	_, _ = h.enforcer.AddGroupingPolicy("cr-already-added", "component:create", "*")
	_, _ = h.enforcer.AddGroupingPolicy("cr-already-added", "component:view", "*")

	old := &authzv1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "cr-already-added", Generation: 1},
		Spec:       authzv1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:view"}},
	}
	newR := &authzv1alpha1.ClusterAuthzRole{
		ObjectMeta: metav1.ObjectMeta{Name: "cr-already-added", Generation: 2},
		Spec:       authzv1alpha1.ClusterAuthzRoleSpec{Actions: []string{"component:create"}},
	}
	require.NoError(t, h.handleUpdateClusterRole(old, newR), "handleUpdateClusterRole should succeed even when new action already exists")
}

// --- handleUpdateBinding — formatSubject errors for old and new binding ---

func TestAuthzInformerHandler_HandleUpdateBinding_OldFormatSubjectError(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	old := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-b-old-claim", Namespace: "acme", Generation: 1},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "", Value: "devs"}, // empty claim → formatSubject error
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       authzv1alpha1.EffectAllow,
		},
	}
	newB := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-b-old-claim", Namespace: "acme", Generation: 2},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "editor"}}},
			Effect:       authzv1alpha1.EffectAllow,
		},
	}
	err := h.handleUpdateBinding(old, newB)
	require.Error(t, err, "handleUpdateBinding with empty old claim should return error")
}

func TestAuthzInformerHandler_HandleUpdateBinding_NewFormatSubjectError(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	old := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-b-new-claim", Namespace: "acme", Generation: 1},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"}, // valid
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "viewer"}}},
			Effect:       authzv1alpha1.EffectAllow,
		},
	}
	newB := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-b-new-claim", Namespace: "acme", Generation: 2},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement:  authzv1alpha1.EntitlementClaim{Claim: "", Value: "devs"}, // empty claim → formatSubject error
			RoleMappings: []authzv1alpha1.RoleMapping{{RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindAuthzRole, Name: "editor"}}},
			Effect:       authzv1alpha1.EffectAllow,
		},
	}
	err := h.handleUpdateBinding(old, newB)
	require.Error(t, err, "handleUpdateBinding with empty new claim should return error")
}

// --- handleUpdateClusterBinding — formatSubject errors for old and new binding ---

func TestAuthzInformerHandler_HandleUpdateClusterBinding_OldFormatSubjectError(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	old := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-cb-old-claim", Generation: 1},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "", Value: "admins"}, // empty claim
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	newB := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-cb-old-claim", Generation: 2},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "editor"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	err := h.handleUpdateClusterBinding(old, newB)
	require.Error(t, err, "handleUpdateClusterBinding with empty old claim should return error")
}

func TestAuthzInformerHandler_HandleUpdateClusterBinding_NewFormatSubjectError(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeClusterAuthzRoleBinding)
	old := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-cb-new-claim", Generation: 1},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"}, // valid
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	newB := &authzv1alpha1.ClusterAuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "upd-cb-new-claim", Generation: 2},
		Spec: authzv1alpha1.ClusterAuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "", Value: "admins"}, // empty claim
			RoleMappings: []authzv1alpha1.ClusterRoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, Name: "editor"},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	err := h.handleUpdateClusterBinding(old, newB)
	require.Error(t, err, "handleUpdateClusterBinding with empty new claim should return error")
}

// --- handleAddBinding — ClusterAuthzRole reference (roleNamespace = "*" branch) ---

func TestAuthzInformerHandler_HandleAddBinding_ClusterRoleRef(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	// A namespaced binding that references a ClusterAuthzRole triggers `roleNamespace = "*"`
	binding := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-binding-cluster-ref", Namespace: "acme"},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{
					Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, // ← triggers roleNamespace = "*"
					Name: "global-viewer",
				},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	require.NoError(t, h.handleAddBinding(binding), "handleAddBinding with ClusterAuthzRole ref error")
	// The policy tuple must use roleNamespace="*" (not "acme") because the RoleRef is ClusterAuthzRole
	hasP, _ := h.enforcer.HasPolicy("groups:devs", "ns/acme", "global-viewer", "*", "allow", "{}", "ns-binding-cluster-ref")
	require.True(t, hasP, "expected policy with roleNamespace=\"*\" when AuthzRoleBinding references a ClusterAuthzRole")
}

// --- handleDeleteBinding — ClusterAuthzRole reference (roleNamespace = "*" branch) ---

func TestAuthzInformerHandler_HandleDeleteBinding_ClusterRoleRef(t *testing.T) {
	h, _ := setupTestHandler(t, CRDTypeAuthzRoleBinding)
	binding := &authzv1alpha1.AuthzRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "del-ns-binding-cluster-ref", Namespace: "acme"},
		Spec: authzv1alpha1.AuthzRoleBindingSpec{
			Entitlement: authzv1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
			RoleMappings: []authzv1alpha1.RoleMapping{{
				RoleRef: authzv1alpha1.RoleRef{
					Kind: authzv1alpha1.RoleRefKindClusterAuthzRole, // ← triggers roleNamespace = "*"
					Name: "global-viewer",
				},
			}},
			Effect: authzv1alpha1.EffectAllow,
		},
	}
	// Seed the policy with roleNamespace="*" (as it would have been stored on add)
	_, _ = h.enforcer.AddPoliciesEx([][]string{{"groups:devs", "ns/acme", "global-viewer", "*", "allow", "{}", "del-ns-binding-cluster-ref"}})
	require.NoError(t, h.handleDeleteBinding(binding), "handleDeleteBinding with ClusterAuthzRole ref error")
	// Verify the policy with roleNamespace="*" was removed
	hasP, _ := h.enforcer.HasPolicy("groups:devs", "ns/acme", "global-viewer", "*", "allow", "{}", "del-ns-binding-cluster-ref")
	require.False(t, hasP, "policy with roleNamespace=\"*\" should be removed after handleDeleteBinding with ClusterAuthzRole ref")
}
