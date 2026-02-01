// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testRoleName         = "test-role"
	testEntitlementType  = "group"
	testEntitlementValue = "test-group"
	user                 = "user"
)

var (
	testScheme     *runtime.Scheme
	testSchemeOnce sync.Once
)

// getTestScheme returns the test scheme, initializing it once on first call
func getTestScheme() *runtime.Scheme {
	testSchemeOnce.Do(func() {
		testScheme = runtime.NewScheme()
		if err := openchoreov1alpha1.AddToScheme(testScheme); err != nil {
			panic("failed to add OpenChoreo scheme: " + err.Error())
		}
	})
	return testScheme
}

// setupTestEnforcer creates a test CasbinEnforcer with fake Kubernetes client
func setupTestEnforcer(t *testing.T) *CasbinEnforcer {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create a fake Kubernetes client using the shared scheme
	fakeClient := fake.NewClientBuilder().WithScheme(getTestScheme()).Build()

	// Create enforcer with fake K8s client
	config := CasbinConfig{
		CacheEnabled: false,
		K8sClient:    fakeClient,
	}

	enforcer, err := NewCasbinEnforcer(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}

	return enforcer
}

// syncGroupingPolicies directly adds role grouping policies to Casbin enforcer.
// Format: [role, action, namespace] where namespace is "*" for cluster roles.
func syncGroupingPolicies(t *testing.T, enforcer *CasbinEnforcer, policies [][]string) {
	t.Helper()
	if len(policies) > 0 {
		_, err := enforcer.enforcer.AddGroupingPolicies(policies)
		if err != nil {
			t.Fatalf("failed to add grouping policies: %v", err)
		}
	}
}

// syncPolicies directly adds policies to Casbin enforcer.
// Format: [subject, resource_path, role, role_namespace, effect, context]
func syncPolicies(t *testing.T, enforcer *CasbinEnforcer, policies [][]string) {
	t.Helper()
	if len(policies) > 0 {
		_, err := enforcer.enforcer.AddPolicies(policies)
		if err != nil {
			t.Fatalf("failed to add policies: %v", err)
		}
	}
}

// TestCasbinEnforcer_Evaluate_ClusterRoles_Focused tests authorization with cluster-scoped roles only
func TestCasbinEnforcer_Evaluate_ClusterRoles_Focused(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	syncGroupingPolicies(t, enforcer, [][]string{
		{"multi-role", "namespace:view", "*"},
		{"multi-role", "component:*", "*"},
		{"multi-role", "project:view", "*"},
		{"global-admin", "*", "*"},
		{"component-deployer", "component:deploy", "*"},
		{"project-admin", "project:*", "*"},
		{"project-admin", "component:create", "*"},
		{"reader", "component:view", "*"},
		{"reader", "project:view", "*"},
		{"writer", "component:create", "*"},
		{"writer", "project:create", "*"},
		{"sub-claim-role", "component:view", "*"},
		{"service-account-role", "component:deploy", "*"},
		{"service-account-role", "component:view", "*"},
	})

	syncPolicies(t, enforcer, [][]string{
		{"groups:test-group", "ns/acme", "multi-role", "*", "allow", "{}"},
		{"groups:global-admin-group", "*", "global-admin", "*", "allow", "{}"},
		{"groups:component-group", "ns/acme/project/p1/component/c1", "component-deployer", "*", "allow", "{}"},
		{"groups:project-group", "ns/acme/project/p2", "project-admin", "*", "allow", "{}"},
		{"groups:multi-role-group", "ns/acme", "reader", "*", "allow", "{}"},
		{"groups:multi-role-group", "ns/acme", "writer", "*", "allow", "{}"},
		{"sub:user-123", "ns/acme", "sub-claim-role", "*", "allow", "{}"},
		{"groups:sa-group", "ns/acme", "service-account-role", "*", "allow", "{}"},
	})

	tests := []struct {
		name              string
		subjectType       string
		entitlementClaim  string
		entitlementValues []string
		resource          authzcore.ResourceHierarchy
		action            string
		want              bool
		reason            string
	}{
		{
			name:              "cluster role - basic authorization",
			entitlementValues: []string{"test-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme"},
			action:            "namespace:view",
			want:              true,
			reason:            "namespace:* should match namespace:view",
		},
		{
			name:              "cluster role - hierarchical matching",
			entitlementValues: []string{"test-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:view",
			want:              true,
			reason:            "policy at namespace level should apply to child resources",
		},
		{
			name:              "cluster role - wildcard action matching",
			entitlementValues: []string{"test-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:deploy",
			want:              true,
			reason:            "component:* should match component:deploy",
		},
		{
			name:              "cluster role - action not permitted",
			entitlementValues: []string{"test-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1"},
			action:            "project:delete",
			want:              false,
			reason:            "project:delete not in role actions",
		},
		{
			name:              "cluster role - no matching group",
			entitlementValues: []string{"wrong-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Component: "c1"},
			action:            "component:view",
			want:              false,
			reason:            "user group doesn't match any policy",
		},
		{
			name:              "cluster role - hierarchy out of scope",
			entitlementValues: []string{"test-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "other-namespace", Project: "p1"},
			action:            "component:view",
			want:              false,
			reason:            "policy scoped to 'acme', not 'other-namespace'",
		},
		{
			name:              "cluster role - multiple groups, one matches",
			entitlementValues: []string{"group1", "test-group", "group2"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme"},
			action:            "component:view",
			want:              true,
			reason:            "at least one group matches the policy",
		},
		{
			name:              "global wildcard - access any namespace",
			entitlementValues: []string{"global-admin-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "any-namespace", Project: "any-project"},
			action:            "project:delete",
			want:              true,
			reason:            "global wildcard grants access to all resources",
		},
		{
			name:              "component-level policy - exact match allowed",
			entitlementValues: []string{"component-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:deploy",
			want:              true,
			reason:            "exact component match should allow access",
		},
		{
			name:              "component-level policy - different component denied",
			entitlementValues: []string{"component-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c2"},
			action:            "component:deploy",
			want:              false,
			reason:            "policy scoped to c1, not c2",
		},
		{
			name:              "project-level policy - applies to children",
			entitlementValues: []string{"project-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p2", Component: "c1"},
			action:            "component:create",
			want:              true,
			reason:            "project-level policy applies to components within project",
		},
		{
			name:              "multiple roles - read permission",
			entitlementValues: []string{"multi-role-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:view",
			want:              true,
			reason:            "reader role provides view permission",
		},
		{
			name:              "multiple roles - write permission",
			entitlementValues: []string{"multi-role-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:create",
			want:              true,
			reason:            "writer role provides create permission",
		},
		{
			name:              "path matching - no false positive for similar names",
			entitlementValues: []string{"test-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme2"},
			action:            "namespace:view",
			want:              false,
			reason:            "'acme' policy should not match 'acme2'",
		},
		{
			name:              "service account - deploy permission",
			subjectType:       "service_account",
			entitlementValues: []string{"sa-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:deploy",
			want:              true,
			reason:            "service account should be able to deploy components",
		},
		{
			name:              "sub claim - authorized user",
			entitlementClaim:  "sub",
			entitlementValues: []string{"user-123"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:view",
			want:              true,
			reason:            "'sub' claim should work for authorization",
		},
		{
			name:              "sub claim - unauthorized user",
			entitlementClaim:  "sub",
			entitlementValues: []string{"user-456"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Component: "c1"},
			action:            "component:view",
			want:              false,
			reason:            "different sub value should be denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subjectType := tt.subjectType
			if subjectType == "" {
				subjectType = "user"
			}
			entitlementClaim := tt.entitlementClaim
			if entitlementClaim == "" {
				entitlementClaim = "groups"
			}

			request := &authzcore.EvaluateRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              subjectType,
					EntitlementClaim:  entitlementClaim,
					EntitlementValues: tt.entitlementValues,
				},
				Resource: authzcore.Resource{
					Type:      "test-resource",
					Hierarchy: tt.resource,
				},
				Action: tt.action,
			}

			decision, err := enforcer.Evaluate(ctx, request)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}

			if decision.Decision != tt.want {
				t.Errorf("Evaluate() decision = %v, want %v\nExpected: %s\nActual: %s",
					decision.Decision, tt.want, tt.reason, decision.Context.Reason)
			}
		})
	}
}

// TestCasbinEnforcer_Evaluate_NamespaceRoles_Focused tests authorization with namespace-scoped roles only
func TestCasbinEnforcer_Evaluate_NamespaceRoles_Focused(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	syncGroupingPolicies(t, enforcer, [][]string{
		{"ns-engineer", "component:deploy", "acme"},
		{"ns-engineer", "component:view", "acme"},
		{"ns-engineer", "project:view", "acme"},
		{"ns-project-lead", "project:*", "acme"},
		{"ns-project-lead", "component:*", "acme"},
		{"ns-reader", "component:view", "acme"},
		{"ns-reader", "project:view", "acme"},
		{"ns-writer", "component:create", "acme"},
		{"ns-writer", "project:create", "acme"},
	})

	syncPolicies(t, enforcer, [][]string{
		{"groups:ns-engineer-group", "ns/acme", "ns-engineer", "acme", "allow", "{}"},
		{"groups:ns-project-lead-group", "ns/acme/project/p1", "ns-project-lead", "acme", "allow", "{}"},
		{"groups:ns-multi-role-group", "ns/acme", "ns-reader", "acme", "allow", "{}"},
		{"groups:ns-multi-role-group", "ns/acme", "ns-writer", "acme", "allow", "{}"},
	})

	tests := []struct {
		name              string
		entitlementValues []string
		resource          authzcore.ResourceHierarchy
		action            string
		want              bool
		reason            string
	}{
		{
			name:              "namespace role - basic access in own namespace",
			entitlementValues: []string{"ns-engineer-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:deploy",
			want:              true,
			reason:            "namespace role should grant deploy access within its namespace",
		},
		{
			name:              "namespace role - wildcard action matching",
			entitlementValues: []string{"ns-project-lead-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:delete",
			want:              true,
			reason:            "component:* should match component:delete",
		},
		{
			name:              "namespace role - action not in role",
			entitlementValues: []string{"ns-engineer-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:delete",
			want:              false,
			reason:            "namespace role doesn't have delete action",
		},
		{
			name:              "namespace role - project-level scope works",
			entitlementValues: []string{"ns-project-lead-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:create",
			want:              true,
			reason:            "project-level scoped namespace role applies to components in that project",
		},
		{
			name:              "namespace role - denied outside mapped project",
			entitlementValues: []string{"ns-project-lead-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p2"},
			action:            "project:delete",
			want:              false,
			reason:            "namespace role mapped to p1 should not work for p2",
		},
		{
			name:              "multiple namespace roles - read permission",
			entitlementValues: []string{"ns-multi-role-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Component: "c1"},
			action:            "component:view",
			want:              true,
			reason:            "user has view permission from ns-reader role",
		},
		{
			name:              "multiple namespace roles - write permission",
			entitlementValues: []string{"ns-multi-role-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Component: "c1"},
			action:            "component:create",
			want:              true,
			reason:            "user has create permission from ns-writer role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &authzcore.EvaluateRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              "user",
					EntitlementClaim:  "groups",
					EntitlementValues: tt.entitlementValues,
				},
				Resource: authzcore.Resource{
					Type:      "test-resource",
					Hierarchy: tt.resource,
				},
				Action: tt.action,
			}

			decision, err := enforcer.Evaluate(ctx, request)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}

			if decision.Decision != tt.want {
				t.Errorf("Evaluate() decision = %v, want %v\nExpected: %s\nActual: %s",
					decision.Decision, tt.want, tt.reason, decision.Context.Reason)
			}
		})
	}
}

// TestCasbinEnforcer_Evaluate_DenyPolicies_Focused tests deny policy logic for both cluster and namespace roles
func TestCasbinEnforcer_Evaluate_DenyPolicies_Focused(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	syncGroupingPolicies(t, enforcer, [][]string{
		// Cluster role
		{"developer", "component:*", "*"},
		{"developer", "project:view", "*"},
		// Namespace role
		{"ns-developer", "component:*", "acme"},
		{"ns-developer", "project:*", "acme"},
	})

	syncPolicies(t, enforcer, [][]string{
		// Cluster role: allow at namespace level
		{"groups:user-group", "ns/acme", "developer", "*", "allow", "{}"},
		// Cluster role: deny at project level
		{"groups:user-group", "ns/acme/project/secret", "developer", "*", "deny", "{}"},
		// Namespace role: allow at namespace level
		{"groups:ns-user-group", "ns/acme", "ns-developer", "acme", "allow", "{}"},
		// Namespace role: deny at component level
		{"groups:ns-user-group", "ns/acme/project/p1/component/restricted", "ns-developer", "acme", "deny", "{}"},
		// Cross-role deny: namespace role deny for cluster role user
		{"groups:user-group", "ns/acme/project/public/component/forbidden", "ns-developer", "acme", "deny", "{}"},
	})

	tests := []struct {
		name              string
		entitlementValues []string
		resource          authzcore.ResourceHierarchy
		action            string
		want              bool
		reason            string
	}{
		{
			name:              "cluster role - allow in public project",
			entitlementValues: []string{"user-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "public", Component: "c1"},
			action:            "component:view",
			want:              true,
			reason:            "allow policy at namespace level permits access to public project",
		},
		{
			name:              "cluster role - deny overrides allow",
			entitlementValues: []string{"user-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "secret", Component: "c1"},
			action:            "component:view",
			want:              false,
			reason:            "deny policy at project level overrides allow policy at namespace level",
		},
		{
			name:              "namespace role - allow in normal component",
			entitlementValues: []string{"ns-user-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "normal"},
			action:            "component:deploy",
			want:              true,
			reason:            "namespace role allow at namespace level permits access",
		},
		{
			name:              "namespace role - deny overrides allow",
			entitlementValues: []string{"ns-user-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "restricted"},
			action:            "component:deploy",
			want:              false,
			reason:            "namespace role deny at component level overrides namespace-level allow",
		},
		{
			name:              "cross-role - namespace deny overrides cluster allow",
			entitlementValues: []string{"user-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "public", Component: "forbidden"},
			action:            "component:view",
			want:              false,
			reason:            "namespace role deny should override cluster role allow",
		},
		{
			name:              "cross-role - other components allowed",
			entitlementValues: []string{"user-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "public", Component: "allowed"},
			action:            "component:view",
			want:              true,
			reason:            "deny scoped to 'forbidden' component, 'allowed' component should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &authzcore.EvaluateRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              "user",
					EntitlementClaim:  "groups",
					EntitlementValues: tt.entitlementValues,
				},
				Resource: authzcore.Resource{
					Type:      "test-resource",
					Hierarchy: tt.resource,
				},
				Action: tt.action,
			}

			decision, err := enforcer.Evaluate(ctx, request)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}

			if decision.Decision != tt.want {
				t.Errorf("Evaluate() decision = %v, want %v\nExpected: %s\nActual: %s",
					decision.Decision, tt.want, tt.reason, decision.Context.Reason)
			}
		})
	}
}

// TestCasbinEnforcer_Evaluate_CrossNamespaceIsolation_Focused tests that namespace roles don't leak across namespaces
func TestCasbinEnforcer_Evaluate_CrossNamespaceIsolation_Focused(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	syncGroupingPolicies(t, enforcer, [][]string{
		// ns-engineer in 'acme' namespace
		{"ns-engineer", "component:deploy", "acme"},
		{"ns-engineer", "component:view", "acme"},
		{"ns-engineer", "project:view", "acme"},
		// ns-engineer in 'other-namespace' (same role name, different actions)
		{"ns-engineer", "component:delete", "other-namespace"},
		{"ns-engineer", "project:delete", "other-namespace"},
	})

	syncPolicies(t, enforcer, [][]string{
		{"groups:acme-engineer-group", "ns/acme", "ns-engineer", "acme", "allow", "{}"},
		{"groups:other-engineer-group", "ns/other-namespace", "ns-engineer", "other-namespace", "allow", "{}"},
	})

	tests := []struct {
		name              string
		entitlementValues []string
		resource          authzcore.ResourceHierarchy
		action            string
		want              bool
		reason            string
	}{
		{
			name:              "acme role - works in own namespace",
			entitlementValues: []string{"acme-engineer-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:deploy",
			want:              true,
			reason:            "namespace role should work in its own namespace",
		},
		{
			name:              "acme role - no access to other namespace",
			entitlementValues: []string{"acme-engineer-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "other-namespace", Project: "p1", Component: "c1"},
			action:            "component:deploy",
			want:              false,
			reason:            "namespace role for 'acme' should NOT grant access to 'other-namespace'",
		},
		{
			name:              "other-namespace role - works in own namespace",
			entitlementValues: []string{"other-engineer-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "other-namespace", Project: "p1", Component: "c1"},
			action:            "component:delete",
			want:              true,
			reason:            "namespace role should work in its own namespace",
		},
		{
			name:              "other-namespace role - no access to acme namespace",
			entitlementValues: []string{"other-engineer-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1"},
			action:            "project:delete",
			want:              false,
			reason:            "namespace role for 'other-namespace' should NOT grant access to 'acme'",
		},
		{
			name:              "same role name - acme permissions don't leak to other-namespace",
			entitlementValues: []string{"acme-engineer-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:delete",
			want:              false,
			reason:            "acme ns-engineer role doesn't have delete (only other-namespace does)",
		},
		{
			name:              "same role name - other-namespace permissions don't leak to acme",
			entitlementValues: []string{"other-engineer-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "other-namespace", Project: "p1", Component: "c1"},
			action:            "component:deploy",
			want:              false,
			reason:            "other-namespace ns-engineer role doesn't have deploy (only acme does)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &authzcore.EvaluateRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              "user",
					EntitlementClaim:  "groups",
					EntitlementValues: tt.entitlementValues,
				},
				Resource: authzcore.Resource{
					Type:      "test-resource",
					Hierarchy: tt.resource,
				},
				Action: tt.action,
			}

			decision, err := enforcer.Evaluate(ctx, request)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}

			if decision.Decision != tt.want {
				t.Errorf("Evaluate() decision = %v, want %v\nExpected: %s\nActual: %s",
					decision.Decision, tt.want, tt.reason, decision.Context.Reason)
			}
		})
	}
}

// TestCasbinEnforcer_Evaluate_RoleInteractions_Focused tests interaction between cluster and namespace roles
func TestCasbinEnforcer_Evaluate_RoleInteractions_Focused(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	syncGroupingPolicies(t, enforcer, [][]string{
		// Cluster role "developer"
		{"developer", "component:view", "*"},
		{"developer", "project:view", "*"},
		// Namespace role "developer" in acme (same name, different namespace)
		{"developer", "component:deploy", "acme"},
		{"developer", "component:view", "acme"},
		{"developer", "project:view", "acme"},
		// Cluster role "viewer"
		{"viewer", "component:view", "*"},
		{"viewer", "project:view", "*"},
		// Namespace role "deployer" in acme
		{"deployer", "component:deploy", "acme"},
	})

	syncPolicies(t, enforcer, [][]string{
		// Cluster role mapping for cluster-users
		{"groups:cluster-users", "ns/acme", "developer", "*", "allow", "{}"},
		// Namespace role mapping for ns-users
		{"groups:ns-users", "ns/acme", "developer", "acme", "allow", "{}"},
		// Same group "engineering" mapped to both cluster viewer and namespace deployer
		{"groups:engineering", "ns/acme", "viewer", "*", "allow", "{}"},
		{"groups:engineering", "ns/acme", "deployer", "acme", "allow", "{}"},
	})

	tests := []struct {
		name              string
		entitlementValues []string
		resource          authzcore.ResourceHierarchy
		action            string
		want              bool
		reason            string
	}{
		{
			name:              "user with both roles - cluster role permission",
			entitlementValues: []string{"cluster-users", "ns-users"},
			resource: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Component: "c1",
			},
			action: "component:view",
			want:   true,
			reason: "user with both roles should have view from cluster role",
		},
		{
			name:              "user with both roles - namespace role permission",
			entitlementValues: []string{"cluster-users", "ns-users"},
			resource: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Component: "c1",
			},
			action: "component:deploy",
			want:   true,
			reason: "user with both roles should have deploy from namespace role",
		},
		{
			name:              "user with both roles - neither has permission",
			entitlementValues: []string{"cluster-users", "ns-users"},
			resource: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Component: "c1",
			},
			action: "component:delete",
			want:   false,
			reason: "user with both roles should not have delete (neither role has it)",
		},
		{
			name:              "same group both roles - view from cluster role",
			entitlementValues: []string{"engineering"},
			resource: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Component: "c1",
			},
			action: "component:view",
			want:   true,
			reason: "user in 'engineering' should have view permission from cluster 'viewer' role",
		},
		{
			name:              "same group both roles - cluster role works in acme",
			entitlementValues: []string{"engineering"},
			resource: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p2",
			},
			action: "project:view",
			want:   true,
			reason: "cluster 'viewer' role should work anywhere it's mapped",
		},
		{
			name:              "same group both roles - namespace role limited to acme",
			entitlementValues: []string{"engineering"},
			resource: authzcore.ResourceHierarchy{
				Namespace: "other-namespace",
				Project:   "p1",
				Component: "c1",
			},
			action: "component:deploy",
			want:   false,
			reason: "namespace 'deployer' role limited to 'acme', should NOT work in 'other-namespace'",
		},
		{
			name:              "same group both roles - cluster role should work in other-namespace",
			entitlementValues: []string{"engineering"},
			resource: authzcore.ResourceHierarchy{
				Namespace: "other-namespace",
				Project:   "p1",
			},
			action: "project:view",
			want:   false,
			reason: "cluster role mapped only to 'acme' namespace, not 'other-namespace'",
		},
		{
			name:              "same group both roles - neither has delete permission",
			entitlementValues: []string{"engineering"},
			resource: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Component: "c1",
			},
			action: "component:delete",
			want:   false,
			reason: "neither cluster 'viewer' nor namespace 'deployer' has delete permission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &authzcore.EvaluateRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              "user",
					EntitlementClaim:  "groups",
					EntitlementValues: tt.entitlementValues,
				},
				Resource: authzcore.Resource{
					Type:      "some-resource-type",
					Hierarchy: tt.resource,
				},
				Action: tt.action,
			}

			decision, err := enforcer.Evaluate(ctx, request)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}

			if decision.Decision != tt.want {
				t.Errorf("Evaluate() decision = %v, want %v\nExpected: %s\nActual decision context: %s",
					decision.Decision, tt.want, tt.reason, decision.Context.Reason)
			}
		})
	}
}

// TestCasbinEnforcer_BatchEvaluate tests the BatchEvaluate method
func TestCasbinEnforcer_BatchEvaluate(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	syncGroupingPolicies(t, enforcer, [][]string{
		{"reader", "component:view", "*"},
		{"writer", "component:create", "*"},
	})

	syncPolicies(t, enforcer, [][]string{
		{"groups:dev-group", "ns/acme", "reader", "*", "allow", "{}"},
		{"groups:dev-group", "ns/acme/project/p1", "writer", "*", "allow", "{}"},
	})

	batchRequest := &authzcore.BatchEvaluateRequest{
		Requests: []authzcore.EvaluateRequest{
			{
				SubjectContext: &authzcore.SubjectContext{
					Type:              "user",
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"dev-group"},
				},
				Resource: authzcore.Resource{
					Type: "component",
					Hierarchy: authzcore.ResourceHierarchy{
						Namespace: "acme",
						Project:   "p1",
					},
				},
				Action: "component:view",
			},
			{
				SubjectContext: &authzcore.SubjectContext{
					Type:              "user",
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"dev-group"},
				},
				Resource: authzcore.Resource{
					Type: "component",
					Hierarchy: authzcore.ResourceHierarchy{
						Namespace: "acme",
						Project:   "p1",
					},
				},
				Action: "component:create",
			},
			{
				SubjectContext: &authzcore.SubjectContext{
					Type:              "user",
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"dev-group"},
				},
				Resource: authzcore.Resource{
					Type: "component",
					Hierarchy: authzcore.ResourceHierarchy{
						Namespace: "acme",
						Project:   "p2",
					},
				},
				Action: "component:create",
			},
		},
	}

	response, err := enforcer.BatchEvaluate(ctx, batchRequest)
	if err != nil {
		t.Fatalf("BatchEvaluate() error = %v", err)
	}

	if len(response.Decisions) != 3 {
		t.Fatalf("BatchEvaluate() returned %d decisions, want 3", len(response.Decisions))
	}

	// Verify we got 3 decisions
	expectedResults := []bool{
		true,
		true,
		false,
	}

	for i, expected := range expectedResults {
		if response.Decisions[i].Decision != expected {
			t.Errorf("BatchEvaluate() decision[%d] = %v, want %v (reason: %s)",
				i, response.Decisions[i].Decision, expected, response.Decisions[i].Context.Reason)
		}
	}
}

func TestCasbinEnforcer_filterPoliciesBySubjectAndScope(t *testing.T) {
	enforcer := setupTestEnforcer(t)

	syncGroupingPolicies(t, enforcer, [][]string{
		{"viewer", "component:view", "*"},
		{"editor", "component:edit", "acme"},
	})

	syncPolicies(t, enforcer, [][]string{
		{"group:group1", "ns/acme", "viewer", "*", "allow", "{}"},
		{"group:group1", "ns/acme/project/p1", "viewer", "*", "deny", "{}"},
		{"group:group1", "ns/other-namespace", "viewer", "*", "allow", "{}"},
		{"group:group1", "ns/acme", "editor", "acme", "allow", "{}"},
	})

	tests := []struct {
		name            string
		subjectCtx      *authzcore.SubjectContext
		scopePath       string
		wantPolicyCount int
	}{
		{
			name: "filter policies within scope",
			subjectCtx: &authzcore.SubjectContext{
				Type:              user,
				EntitlementClaim:  "group",
				EntitlementValues: []string{"group1"},
			},
			scopePath:       "ns/acme",
			wantPolicyCount: 3,
		},
		{
			name: "filter policies within project scope",
			subjectCtx: &authzcore.SubjectContext{
				Type:              user,
				EntitlementClaim:  "group",
				EntitlementValues: []string{"group1"},
			},
			scopePath:       "ns/acme/project/p1",
			wantPolicyCount: 3,
		},
		{
			name: "no matching entitlements",
			subjectCtx: &authzcore.SubjectContext{
				Type:              user,
				EntitlementClaim:  "group",
				EntitlementValues: []string{"nonexistent-group"},
			},
			scopePath:       "ns/acme",
			wantPolicyCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policies, err := enforcer.filterPoliciesBySubjectAndScope(tt.subjectCtx, tt.scopePath)
			if err != nil {
				t.Fatalf("filterPoliciesBySubjectAndScope() error = %v", err)
			}
			if len(policies) != tt.wantPolicyCount {
				t.Errorf("filterPoliciesBySubjectAndScope() returned %d policies, want %d", len(policies), tt.wantPolicyCount)
			}
		})
	}
}

func TestCasbinEnforcer_buildCapabilitiesFromPolicies(t *testing.T) {
	enforcer := setupTestEnforcer(t)

	syncGroupingPolicies(t, enforcer, [][]string{
		{"viewer", "component:view", "*"},
		{"viewer", "project:view", "*"},
		{"editor", "component:*", "*"},
		{"admin", "*", "*"},
		{"editor", "component:view", "acme"},
	})

	// Create action index for testing
	testActions := []authzcore.Action{
		{Name: "component:view"},
		{Name: "component:create"},
		{Name: "component:update"},
		{Name: "component:delete"},
		{Name: "project:view"},
		{Name: "project:create"},
		{Name: "namespace:view"},
	}
	actionIdx := indexActions(testActions)

	tests := []struct {
		name                 string
		policies             []policyInfo
		expectedCapabilities map[string]struct {
			allowedCount int
			deniedCount  int
		}
	}{
		{
			name: "multiple roles with different policies",
			policies: []policyInfo{
				{resourcePath: "ns/acme", roleName: "viewer", roleNamespace: "*", effect: "allow"},
				{resourcePath: "ns/acme/project/p1", roleName: "editor", roleNamespace: "*", effect: "allow"},
			},
			expectedCapabilities: map[string]struct {
				allowedCount int
				deniedCount  int
			}{
				"component:view":   {allowedCount: 2, deniedCount: 0},
				"component:create": {allowedCount: 1, deniedCount: 0},
				"component:update": {allowedCount: 1, deniedCount: 0},
				"component:delete": {allowedCount: 1, deniedCount: 0},
				"project:view":     {allowedCount: 1, deniedCount: 0},
			},
		},
		{
			name: "allow and deny effects on different resources",
			policies: []policyInfo{
				{resourcePath: "ns/acme", roleName: "editor", roleNamespace: "*", effect: "allow"},
				{resourcePath: "ns/acme/project/secret", roleName: "editor", roleNamespace: "*", effect: "deny"},
			},
			expectedCapabilities: map[string]struct {
				allowedCount int
				deniedCount  int
			}{
				"component:view":   {allowedCount: 1, deniedCount: 1},
				"component:create": {allowedCount: 1, deniedCount: 1},
				"component:update": {allowedCount: 1, deniedCount: 1},
				"component:delete": {allowedCount: 1, deniedCount: 1},
			},
		},
		{
			name: "wildcard action expansion",
			policies: []policyInfo{
				{resourcePath: "ns/acme", roleName: "admin", roleNamespace: "*", effect: "allow"},
			},
			expectedCapabilities: map[string]struct {
				allowedCount int
				deniedCount  int
			}{
				"component:view":   {allowedCount: 1, deniedCount: 0},
				"component:create": {allowedCount: 1, deniedCount: 0},
				"component:update": {allowedCount: 1, deniedCount: 0},
				"component:delete": {allowedCount: 1, deniedCount: 0},
				"project:view":     {allowedCount: 1, deniedCount: 0},
				"project:create":   {allowedCount: 1, deniedCount: 0},
				"namespace:view":   {allowedCount: 1, deniedCount: 0},
			},
		},
		{
			name: "multiple policies with same role",
			policies: []policyInfo{
				{resourcePath: "ns/acme", roleName: "viewer", roleNamespace: "*", effect: "allow"},
				{resourcePath: "ns/acme", roleName: "viewer", roleNamespace: "*", effect: "allow"},
			},
			expectedCapabilities: map[string]struct {
				allowedCount int
				deniedCount  int
			}{
				"component:view": {allowedCount: 1, deniedCount: 0},
				"project:view":   {allowedCount: 1, deniedCount: 0},
			},
		},
		{
			name:     "empty policies returns empty capabilities",
			policies: []policyInfo{},
			expectedCapabilities: map[string]struct {
				allowedCount int
				deniedCount  int
			}{},
		},
		{
			name: "namespace role isolation - same role name different namespaces",
			policies: []policyInfo{
				{resourcePath: "ns/acme-v2", roleName: "editor", roleNamespace: "*", effect: "allow"},
				{resourcePath: "ns/acme", roleName: "editor", roleNamespace: "acme", effect: "allow"},
			},
			expectedCapabilities: map[string]struct {
				allowedCount int
				deniedCount  int
			}{
				"component:view":   {allowedCount: 2, deniedCount: 0},
				"component:create": {allowedCount: 1, deniedCount: 0},
				"component:update": {allowedCount: 1, deniedCount: 0},
				"component:delete": {allowedCount: 1, deniedCount: 0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capabilities, err := enforcer.buildCapabilitiesFromPolicies(tt.policies, actionIdx)
			if err != nil {
				t.Fatalf("buildCapabilitiesFromPolicies() unexpected error = %v", err)
			}

			if len(capabilities) != len(tt.expectedCapabilities) {
				t.Errorf("buildCapabilitiesFromPolicies() returned %d capabilities, want %d",
					len(capabilities), len(tt.expectedCapabilities))
			}

			for action, expected := range tt.expectedCapabilities {
				cap, ok := capabilities[action]
				if !ok {
					t.Errorf("action %q not found in capabilities", action)
					continue
				}

				if len(cap.Allowed) != expected.allowedCount {
					t.Errorf("action %q: got %d allowed resources, want %d",
						action, len(cap.Allowed), expected.allowedCount)
				}

				if len(cap.Denied) != expected.deniedCount {
					t.Errorf("action %q: got %d denied resources, want %d",
						action, len(cap.Denied), expected.deniedCount)
				}
			}
		})
	}
}

// TestCasbinEnforcer_GetSubjectProfile tests the GetSubjectProfile method
func TestCasbinEnforcer_GetSubjectProfile(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	syncGroupingPolicies(t, enforcer, [][]string{
		// Cluster-scoped roles
		{"viewer", "component:view", "*"},
		{"viewer", "project:view", "*"},
		{"editor", "component:view", "*"},
		{"editor", "component:create", "*"},
		{"editor", "component:update", "*"},
		// Namespace-scoped role
		{"editor", "project:delete", "acme"},
	})

	syncPolicies(t, enforcer, [][]string{
		{"groups:dev-group", "ns/acme", "editor", "*", "allow", "{}"},
		{"groups:dev-group", "ns/acme/project/p1", "viewer", "*", "allow", "{}"},
		{"groups:dev-group", "ns/acme/project/secret", "editor", "*", "deny", "{}"},
		{"groups:dev-group", "ns/acme", "editor", "acme", "allow", "{}"},
	})
	type expectedCapability struct {
		action       string
		allowedCount int
		deniedCount  int
	}

	tests := []struct {
		name                 string
		request              *authzcore.ProfileRequest
		wantErr              bool
		wantEmptyCapability  bool
		expectedUser         authzcore.SubjectContext
		expectedCapabilities []expectedCapability
	}{
		{
			name: "get profile with namespace scope",
			request: &authzcore.ProfileRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              user,
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"dev-group"},
				},
				Scope: authzcore.ResourceHierarchy{
					Namespace: "acme",
				},
			},
			wantErr: false,
			expectedUser: authzcore.SubjectContext{
				Type:              user,
				EntitlementClaim:  "groups",
				EntitlementValues: []string{"dev-group"},
			},
			expectedCapabilities: []expectedCapability{
				{action: "component:view", allowedCount: 2, deniedCount: 1},
				{action: "project:view", allowedCount: 1, deniedCount: 0},
				{action: "component:create", allowedCount: 1, deniedCount: 1},
				{action: "component:update", allowedCount: 1, deniedCount: 1},
				{action: "project:delete", allowedCount: 1, deniedCount: 0},
			},
		},
		{
			name: "get profile for scope within a namespace - no deny policies",
			request: &authzcore.ProfileRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              user,
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"dev-group"},
				},
				Scope: authzcore.ResourceHierarchy{
					Namespace: "acme",
					Project:   "p1",
				},
			},
			wantErr: false,
			expectedUser: authzcore.SubjectContext{
				Type:              user,
				EntitlementClaim:  "groups",
				EntitlementValues: []string{"dev-group"},
			},
			expectedCapabilities: []expectedCapability{
				{action: "component:view", allowedCount: 2, deniedCount: 0},
				{action: "project:view", allowedCount: 1, deniedCount: 0},
				{action: "component:create", allowedCount: 1, deniedCount: 0},
				{action: "component:update", allowedCount: 1, deniedCount: 0},
				{action: "project:delete", allowedCount: 1, deniedCount: 0},
			},
		},
		{
			name: "get profile for user with no permissions",
			request: &authzcore.ProfileRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              user,
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"no-permissions-group"},
				},
				Scope: authzcore.ResourceHierarchy{
					Namespace: "acme",
				},
			},
			wantErr:             false,
			wantEmptyCapability: true,
			expectedUser: authzcore.SubjectContext{
				Type:              user,
				EntitlementClaim:  "groups",
				EntitlementValues: []string{"no-permissions-group"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := enforcer.GetSubjectProfile(ctx, tt.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSubjectProfile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if resp == nil {
				t.Fatal("GetSubjectProfile() returned nil response")
			}
			if resp.GeneratedAt.IsZero() {
				t.Error("expected GeneratedAt to be set")
			}

			// Check the user field
			if resp.User.Type != tt.expectedUser.Type {
				t.Errorf("expected user type %q, got %q", tt.expectedUser.Type, resp.User.Type)
			}
			if resp.User.EntitlementClaim != tt.expectedUser.EntitlementClaim {
				t.Errorf("expected user claim %q, got %q", tt.expectedUser.EntitlementClaim, resp.User.EntitlementClaim)
			}
			if len(resp.User.EntitlementValues) != len(tt.expectedUser.EntitlementValues) {
				t.Errorf("expected %d entitlement values, got %d", len(tt.expectedUser.EntitlementValues), len(resp.User.EntitlementValues))
			}
			for i, expectedVal := range tt.expectedUser.EntitlementValues {
				if resp.User.EntitlementValues[i] != expectedVal {
					t.Errorf("expected entitlement value[%d] %q, got %q", i, expectedVal, resp.User.EntitlementValues[i])
				}
			}

			// Check if we expect empty capabilities
			if tt.wantEmptyCapability {
				if len(resp.Capabilities) != 0 {
					t.Errorf("expected empty capabilities, got %d", len(resp.Capabilities))
				}
				return
			}

			if len(resp.Capabilities) == 0 {
				t.Error("expected capabilities to be non-empty")
			}

			if len(tt.expectedCapabilities) != len(resp.Capabilities) {
				t.Errorf("expected %d capabilities, got %d", len(tt.expectedCapabilities), len(resp.Capabilities))
			}

			for _, exp := range tt.expectedCapabilities {
				cap, ok := resp.Capabilities[exp.action]
				if !ok {
					t.Errorf("expected action %q to be present in capabilities", exp.action)
					continue
				}

				if len(cap.Allowed) != exp.allowedCount {
					t.Errorf("action %q: expected %d allowed resources, got %d", exp.action, exp.allowedCount, len(cap.Allowed))
				}

				if len(cap.Denied) != exp.deniedCount {
					t.Errorf("action %q: expected %d denied resources, got %d", exp.action, exp.deniedCount, len(cap.Denied))
				}
			}
		})
	}
}
