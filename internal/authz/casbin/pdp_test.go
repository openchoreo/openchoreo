// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
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

	enforcer, err := NewEnforcer(context.Background(), config, logger)
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
// Format: [subject, resource_path, role, role_namespace, effect, context, binding_name]
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
		{"groups:test-group", "ns/acme", "multi-role", "*", "allow", "{}", "test-binding-1"},
		{"groups:global-admin-group", "*", "global-admin", "*", "allow", "{}", "global-admin-binding"},
		{"groups:component-group", "ns/acme/project/p1/component/c1", "component-deployer", "*", "allow", "{}", "component-binding"},
		{"groups:project-group", "ns/acme/project/p2", "project-admin", "*", "allow", "{}", "project-binding"},
		{"groups:multi-role-group", "ns/acme", "reader", "*", "allow", "{}", "reader-binding"},
		{"groups:multi-role-group", "ns/acme", "writer", "*", "allow", "{}", "writer-binding"},
		{"sub:user-123", "ns/acme", "sub-claim-role", "*", "allow", "{}", "sub-claim-binding"},
		{"groups:sa-group", "ns/acme", "service-account-role", "*", "allow", "{}", "sa-binding"},
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
		{"groups:ns-engineer-group", "ns/acme", "ns-engineer", "acme", "allow", "{}", "ns-engineer-binding"},
		{"groups:ns-project-lead-group", "ns/acme/project/p1", "ns-project-lead", "acme", "allow", "{}", "ns-project-lead-binding"},
		{"groups:ns-multi-role-group", "ns/acme", "ns-reader", "acme", "allow", "{}", "ns-reader-binding"},
		{"groups:ns-multi-role-group", "ns/acme", "ns-writer", "acme", "allow", "{}", "ns-writer-binding"},
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
		{"groups:user-group", "ns/acme", "developer", "*", "allow", "{}", "developer-allow-binding"},
		// Cluster role: deny at project level
		{"groups:user-group", "ns/acme/project/secret", "developer", "*", "deny", "{}", "developer-deny-binding"},
		// Namespace role: allow at namespace level
		{"groups:ns-user-group", "ns/acme", "ns-developer", "acme", "allow", "{}", "ns-developer-allow-binding"},
		// Namespace role: deny at component level
		{"groups:ns-user-group", "ns/acme/project/p1/component/restricted", "ns-developer", "acme", "deny", "{}", "ns-developer-deny-binding"},
		// Cross-role deny: namespace role deny for cluster role user
		{"groups:user-group", "ns/acme/project/public/component/forbidden", "ns-developer", "acme", "deny", "{}", "cross-role-deny-binding"},
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
		{"groups:acme-engineer-group", "ns/acme", "ns-engineer", "acme", "allow", "{}", "acme-engineer-binding"},
		{"groups:other-engineer-group", "ns/other-namespace", "ns-engineer", "other-namespace", "allow", "{}", "other-engineer-binding"},
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
		{"groups:cluster-users", "ns/acme", "developer", "*", "allow", "{}", "cluster-users-binding"},
		// Namespace role mapping for ns-users
		{"groups:ns-users", "ns/acme", "developer", "acme", "allow", "{}", "ns-users-binding"},
		// Same group "engineering" mapped to both cluster viewer and namespace deployer
		{"groups:engineering", "ns/acme", "viewer", "*", "allow", "{}", "engineering-viewer-binding"},
		{"groups:engineering", "ns/acme", "deployer", "acme", "allow", "{}", "engineering-deployer-binding"},
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
		{"groups:dev-group", "ns/acme", "reader", "*", "allow", "{}", "dev-reader-binding"},
		{"groups:dev-group", "ns/acme/project/p1", "writer", "*", "allow", "{}", "dev-writer-binding"},
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
	condJSON := mustCondsJSON(t, []openchoreov1alpha1.AuthzCondition{
		{Actions: []string{"component:view"}, Expression: `resource.environment == "prod"`},
	})

	enforcer := setupTestEnforcer(t)

	syncGroupingPolicies(t, enforcer, [][]string{
		{"viewer", "component:view", "*"},
		{"editor", "component:edit", "acme"},
	})

	syncPolicies(t, enforcer, [][]string{
		{"group:group1", "ns/acme", "viewer", "*", "allow", "{}", "group1-viewer-acme"},
		{"group:group1", "ns/acme/project/p1", "viewer", "*", "deny", "{}", "group1-viewer-deny"},
		{"group:group1", "ns/other-namespace", "viewer", "*", "allow", "{}", "group1-viewer-other"},
		{"group:group1", "ns/acme", "editor", "acme", "allow", "{}", "group1-editor-acme"},
		{"group:group1", "ns/acme/project/p2", "viewer", "*", "allow", condJSON, "group1-viewer-cond"},
	})

	tests := []struct {
		name            string
		subjectCtx      *authzcore.SubjectContext
		scopePath       string
		wantPolicyCount int
		// conditionsForPath checks that the policyInfo for the given resource path carries the expected conditions string.
		// Empty string means "don't check".
		conditionsForPath map[string]string
	}{
		{
			name: "filter policies within scope",
			subjectCtx: &authzcore.SubjectContext{
				Type:              user,
				EntitlementClaim:  "group",
				EntitlementValues: []string{"group1"},
			},
			scopePath:       "ns/acme",
			wantPolicyCount: 4,
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
		{
			name: "conditions field is propagated verbatim into policyInfo",
			subjectCtx: &authzcore.SubjectContext{
				Type:              user,
				EntitlementClaim:  "group",
				EntitlementValues: []string{"group1"},
			},
			scopePath:       "ns/acme/project/p2",
			wantPolicyCount: 3,
			conditionsForPath: map[string]string{
				"ns/acme/project/p2": condJSON,
				"ns/acme":            "{}",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policies, err := enforcer.filterPoliciesBySubjectAndScope(tt.subjectCtx, tt.scopePath)
			require.NoError(t, err)
			require.Len(t, policies, tt.wantPolicyCount)

			for path, wantConds := range tt.conditionsForPath {
				var found bool
				for _, p := range policies {
					if p.resourcePath == path {
						require.Equal(t, wantConds, p.conditions,
							"policyInfo for path %q: conditions mismatch", path)
						found = true
						break
					}
				}
				require.True(t, found, "no policyInfo found for path %q", path)
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

	// capExpectation describes what we expect for one action's capability.
	type capExpectation struct {
		allowedCount      int
		deniedCount       int
		constraintsByPath map[string][]string
	}

	tests := []struct {
		name                 string
		policies             []policyInfo
		expectedCapabilities map[string]capExpectation
	}{
		{
			name: "multiple roles with different policies",
			policies: []policyInfo{
				{resourcePath: "ns/acme", roleName: "viewer", roleNamespace: "*", effect: "allow"},
				{resourcePath: "ns/acme/project/p1", roleName: "editor", roleNamespace: "*", effect: "allow"},
			},
			expectedCapabilities: map[string]capExpectation{
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
			expectedCapabilities: map[string]capExpectation{
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
			expectedCapabilities: map[string]capExpectation{
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
			expectedCapabilities: map[string]capExpectation{
				"component:view": {allowedCount: 1, deniedCount: 0},
				"project:view":   {allowedCount: 1, deniedCount: 0},
			},
		},
		{
			name:                 "empty policies returns empty capabilities",
			policies:             []policyInfo{},
			expectedCapabilities: map[string]capExpectation{},
		},
		{
			name: "namespace role isolation - same role name different namespaces",
			policies: []policyInfo{
				{resourcePath: "ns/acme-v2", roleName: "editor", roleNamespace: "*", effect: "allow"},
				{resourcePath: "ns/acme", roleName: "editor", roleNamespace: "acme", effect: "allow"},
			},
			expectedCapabilities: map[string]capExpectation{
				"component:view":   {allowedCount: 2, deniedCount: 0},
				"component:create": {allowedCount: 1, deniedCount: 0},
				"component:update": {allowedCount: 1, deniedCount: 0},
				"component:delete": {allowedCount: 1, deniedCount: 0},
			},
		},
		{
			name: "condition on one action constrains it while other actions in the same role remain unconditional",
			policies: []policyInfo{
				{
					resourcePath:  "ns/acme",
					roleName:      "viewer",
					roleNamespace: "*",
					effect:        "allow",
					conditions: mustCondsJSON(t, []openchoreov1alpha1.AuthzCondition{
						{Actions: []string{"component:view"}, Expression: `resource.environment == "prod"`},
					}),
				},
			},
			expectedCapabilities: map[string]capExpectation{
				"component:view": {
					allowedCount: 1,
					deniedCount:  0,
					constraintsByPath: map[string][]string{
						"ns/acme": {`resource.environment == "prod"`},
					},
				},
				"project:view": {
					allowedCount: 1,
					deniedCount:  0,
					constraintsByPath: map[string][]string{
						"ns/acme": nil,
					},
				},
			},
		},
		{
			name: "conditional binding at project scope and unconditional binding at namespace scope are independent",
			policies: []policyInfo{
				{
					resourcePath:  "ns/acme/project/p1",
					roleName:      "viewer",
					roleNamespace: "*",
					effect:        "allow",
					conditions: mustCondsJSON(t, []openchoreov1alpha1.AuthzCondition{
						{Actions: []string{"component:view"}, Expression: `resource.environment == "prod"`},
					}),
				},
				{
					resourcePath:  "ns/acme",
					roleName:      "viewer",
					roleNamespace: "*",
					effect:        "allow",
					conditions:    "{}",
				},
			},
			expectedCapabilities: map[string]capExpectation{
				"component:view": {
					allowedCount: 2,
					deniedCount:  0,
					constraintsByPath: map[string][]string{
						"ns/acme/project/p1": {`resource.environment == "prod"`},
						"ns/acme":            nil,
					},
				},
				"project:view": {
					allowedCount: 2,
					deniedCount:  0,
					constraintsByPath: map[string][]string{
						"ns/acme/project/p1": nil,
						"ns/acme":            nil,
					},
				},
			},
		},
		{
			name: "unconditional binding after conditional wins — conditional expressions discarded",
			policies: []policyInfo{
				{
					resourcePath:  "ns/acme",
					roleName:      "viewer",
					roleNamespace: "*",
					effect:        "allow",
					conditions: mustCondsJSON(t, []openchoreov1alpha1.AuthzCondition{
						{Actions: []string{"component:view"}, Expression: `resource.environment == "prod"`},
					}),
				},
				// Second binding for same role/path is unconditional — should win.
				{
					resourcePath:  "ns/acme",
					roleName:      "viewer",
					roleNamespace: "*",
					effect:        "allow",
					conditions:    "{}",
				},
			},
			expectedCapabilities: map[string]capExpectation{
				"component:view": {
					allowedCount: 1,
					deniedCount:  0,
					constraintsByPath: map[string][]string{
						"ns/acme": nil,
					},
				},
				"project:view": {
					allowedCount: 1,
					deniedCount:  0,
					constraintsByPath: map[string][]string{
						"ns/acme": nil,
					},
				},
			},
		},
		{
			name: "conditional binding after unconditional is ignored — unconditional wins",
			policies: []policyInfo{
				// Unconditional binding processed first.
				{
					resourcePath:  "ns/acme",
					roleName:      "viewer",
					roleNamespace: "*",
					effect:        "allow",
					conditions:    "{}",
				},
				// Conditional binding for same role/path arrives later — must be ignored.
				{
					resourcePath:  "ns/acme",
					roleName:      "viewer",
					roleNamespace: "*",
					effect:        "allow",
					conditions: mustCondsJSON(t, []openchoreov1alpha1.AuthzCondition{
						{Actions: []string{"component:view"}, Expression: `resource.environment == "prod"`},
					}),
				},
			},
			expectedCapabilities: map[string]capExpectation{
				"component:view": {
					allowedCount: 1,
					deniedCount:  0,
					constraintsByPath: map[string][]string{
						"ns/acme": nil,
					},
				},
				"project:view": {
					allowedCount: 1,
					deniedCount:  0,
					constraintsByPath: map[string][]string{
						"ns/acme": nil,
					},
				},
			},
		},
		{
			name: "multiple conditions on same action are OR'd into expressions slice",
			policies: []policyInfo{
				{
					resourcePath:  "ns/acme",
					roleName:      "viewer",
					roleNamespace: "*",
					effect:        "allow",
					conditions: mustCondsJSON(t, []openchoreov1alpha1.AuthzCondition{
						{Actions: []string{"component:view"}, Expression: `resource.environment == "prod"`},
						{Actions: []string{"component:view"}, Expression: `resource.environment == "staging"`},
					}),
				},
			},
			expectedCapabilities: map[string]capExpectation{
				"component:view": {
					allowedCount: 1,
					deniedCount:  0,
					constraintsByPath: map[string][]string{
						"ns/acme": {`resource.environment == "prod"`, `resource.environment == "staging"`},
					},
				},
				// viewer also grants project:view with no condition — unconditional
				"project:view": {
					allowedCount: 1,
					deniedCount:  0,
					constraintsByPath: map[string][]string{
						"ns/acme": nil,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capabilities, err := enforcer.buildCapabilitiesFromPolicies(tt.policies, actionIdx)
			require.NoError(t, err)
			require.Len(t, capabilities, len(tt.expectedCapabilities))

			for action, expected := range tt.expectedCapabilities {
				cap, ok := capabilities[action]
				require.True(t, ok, "action %q not found in capabilities", action)

				require.Len(t, cap.Allowed, expected.allowedCount,
					"action %q: allowed count mismatch", action)
				require.Len(t, cap.Denied, expected.deniedCount,
					"action %q: denied count mismatch", action)

				// Check constraints for specific resource paths when specified.
				for _, res := range cap.Allowed {
					wantExprs, specified := expected.constraintsByPath[res.Path]
					if !specified {
						continue
					}
					if wantExprs == nil {
						require.Nil(t, res.Constraints,
							"action %q path %q: expected nil Constraints", action, res.Path)
					} else {
						require.NotNil(t, res.Constraints,
							"action %q path %q: expected non-nil Constraints", action, res.Path)
						require.ElementsMatch(t, wantExprs, res.Constraints.Expressions,
							"action %q path %q: expressions mismatch", action, res.Path)
					}
				}
			}
		})
	}
}

// TestCasbinEnforcer_GetSubjectProfile tests the GetSubjectProfile method
func TestCasbinEnforcer_GetSubjectProfile(t *testing.T) {
	ctx := context.Background()

	type expectedCapability struct {
		action                  string
		allowedCount            int
		deniedCount             int
		constraintsByPath       map[string][]string
		deniedConstraintsByPath map[string][]string
	}

	baseGroupingPolicies := [][]string{
		{"viewer", "component:view", "*"},
		{"viewer", "project:view", "*"},
		{"editor", "component:view", "*"},
		{"editor", "component:create", "*"},
		{"editor", "component:update", "*"},
		{"editor", "project:delete", "acme"},
	}

	tests := []struct {
		name                 string
		groupingPolicies     [][]string
		policies             [][]string
		request              *authzcore.ProfileRequest
		wantErr              bool
		wantEmptyCapability  bool
		expectedUser         authzcore.SubjectContext
		expectedCapabilities []expectedCapability
	}{
		{
			// Verifies capabilities are returned for a namespace-scoped request with both allow and deny bindings.
			name:             "get profile with namespace scope",
			groupingPolicies: baseGroupingPolicies,
			policies: [][]string{
				{"groups:dev-group", "ns/acme", "editor", "*", "allow", "{}", "dev-editor-allow"},
				{"groups:dev-group", "ns/acme/project/secret", "editor", "*", "deny", "{}", "dev-editor-deny"},
			},
			request: &authzcore.ProfileRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              user,
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"dev-group"},
				},
				Scope: authzcore.ResourceHierarchy{Namespace: "acme"},
			},
			wantErr: false,
			expectedUser: authzcore.SubjectContext{
				Type:              user,
				EntitlementClaim:  "groups",
				EntitlementValues: []string{"dev-group"},
			},
			expectedCapabilities: []expectedCapability{
				{action: "component:view", allowedCount: 1, deniedCount: 1},
				{action: "component:create", allowedCount: 1, deniedCount: 1},
				{action: "component:update", allowedCount: 1, deniedCount: 1},
			},
		},
		{
			// Verifies that deny bindings outside the requested project scope are excluded.
			name:             "get profile scoped to project excludes deny outside scope",
			groupingPolicies: baseGroupingPolicies,
			policies: [][]string{
				{"groups:dev-group", "ns/acme", "editor", "*", "allow", "{}", "dev-editor-allow"},
				{"groups:dev-group", "ns/acme/project/p1", "viewer", "*", "allow", "{}", "dev-viewer-p1"},
				{"groups:dev-group", "ns/acme/project/other", "editor", "*", "deny", "{}", "dev-editor-deny-other"},
			},
			request: &authzcore.ProfileRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              user,
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"dev-group"},
				},
				Scope: authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1"},
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
			},
		},
		{
			// Verifies that a subject with no matching bindings returns empty capabilities.
			name:             "get profile for user with no permissions",
			groupingPolicies: baseGroupingPolicies,
			policies:         [][]string{},
			request: &authzcore.ProfileRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              user,
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"no-permissions-group"},
				},
				Scope: authzcore.ResourceHierarchy{Namespace: "acme"},
			},
			wantErr:             false,
			wantEmptyCapability: true,
			expectedUser: authzcore.SubjectContext{
				Type:              user,
				EntitlementClaim:  "groups",
				EntitlementValues: []string{"no-permissions-group"},
			},
		},
		{
			name:             "conditional allow produces Constraints on matching resource, unconditional on others",
			groupingPolicies: baseGroupingPolicies,
			policies: [][]string{
				{"groups:dev-group", "ns/acme", "viewer", "*", "allow", "{}", "dev-viewer-allow"},
				{"groups:dev-group", "ns/acme/project/p1", "viewer", "*", "allow",
					mustCondsJSON(t, []openchoreov1alpha1.AuthzCondition{
						{Actions: []string{"component:view"}, Expression: `resource.environment == "prod"`},
					}), "dev-viewer-p1-cond"},
			},
			request: &authzcore.ProfileRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              user,
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"dev-group"},
				},
				Scope: authzcore.ResourceHierarchy{Namespace: "acme"},
			},
			wantErr: false,
			expectedUser: authzcore.SubjectContext{
				Type:              user,
				EntitlementClaim:  "groups",
				EntitlementValues: []string{"dev-group"},
			},
			expectedCapabilities: []expectedCapability{
				{
					action:       "component:view",
					allowedCount: 2,
					deniedCount:  0,
					constraintsByPath: map[string][]string{
						"ns/acme":            nil,
						"ns/acme/project/p1": {`resource.environment == "prod"`},
					},
				},
				{
					action:       "project:view",
					allowedCount: 2,
					deniedCount:  0,
					constraintsByPath: map[string][]string{
						"ns/acme":            nil,
						"ns/acme/project/p1": nil,
					},
				},
			},
		},
		{
			name:             "conditional deny produces Constraints on denied resource, unconditional deny has nil Constraints",
			groupingPolicies: baseGroupingPolicies,
			policies: [][]string{
				{"groups:dev-group", "ns/acme", "editor", "*", "allow", "{}", "dev-editor-allow"},
				{"groups:dev-group", "ns/acme/project/secret", "editor", "*", "deny", "{}", "dev-editor-deny"},
				{"groups:dev-group", "ns/acme/project/restricted", "editor", "*", "deny",
					mustCondsJSON(t, []openchoreov1alpha1.AuthzCondition{
						{Actions: []string{"component:view"}, Expression: `resource.environment == "staging"`},
					}), "dev-editor-cond-deny"},
			},
			request: &authzcore.ProfileRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              user,
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"dev-group"},
				},
				Scope: authzcore.ResourceHierarchy{Namespace: "acme"},
			},
			wantErr: false,
			expectedUser: authzcore.SubjectContext{
				Type:              user,
				EntitlementClaim:  "groups",
				EntitlementValues: []string{"dev-group"},
			},
			expectedCapabilities: []expectedCapability{
				{
					action:       "component:view",
					allowedCount: 1,
					deniedCount:  2,
					constraintsByPath: map[string][]string{
						"ns/acme": nil,
					},
					deniedConstraintsByPath: map[string][]string{
						"ns/acme/project/secret":     nil,
						"ns/acme/project/restricted": {`resource.environment == "staging"`},
					},
				},
				{action: "component:create", allowedCount: 1, deniedCount: 2},
				{action: "component:update", allowedCount: 1, deniedCount: 2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enforcer := setupTestEnforcer(t)
			syncGroupingPolicies(t, enforcer, tt.groupingPolicies)
			syncPolicies(t, enforcer, tt.policies)
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

				// Check Constraints per resource path when specified.
				if len(exp.constraintsByPath) > 0 {
					byPath := make(map[string]*authzcore.CapabilityResource, len(cap.Allowed))
					for _, r := range cap.Allowed {
						byPath[r.Path] = r
					}
					for path, wantExprs := range exp.constraintsByPath {
						res, found := byPath[path]
						require.True(t, found, "action %q: expected allowed resource at path %q", exp.action, path)
						if wantExprs == nil {
							require.Nil(t, res.Constraints,
								"action %q path %q: expected nil Constraints (unconditional)", exp.action, path)
						} else {
							require.NotNil(t, res.Constraints,
								"action %q path %q: expected non-nil Constraints", exp.action, path)
							require.ElementsMatch(t, wantExprs, res.Constraints.Expressions,
								"action %q path %q: expressions mismatch", exp.action, path)
						}
					}
				}

				// Check Constraints per denied resource path when specified.
				if len(exp.deniedConstraintsByPath) > 0 {
					byPath := make(map[string]*authzcore.CapabilityResource, len(cap.Denied))
					for _, r := range cap.Denied {
						byPath[r.Path] = r
					}
					for path, wantExprs := range exp.deniedConstraintsByPath {
						res, found := byPath[path]
						require.True(t, found, "action %q: expected denied resource at path %q", exp.action, path)
						if wantExprs == nil {
							require.Nil(t, res.Constraints,
								"action %q path %q: expected nil Constraints (unconditional deny)", exp.action, path)
						} else {
							require.NotNil(t, res.Constraints,
								"action %q path %q: expected non-nil Constraints", exp.action, path)
							require.ElementsMatch(t, wantExprs, res.Constraints.Expressions,
								"action %q path %q: expressions mismatch", exp.action, path)
						}
					}
				}
			}
		})
	}
}

// TestCasbinEnforcer_Evaluate_ScopedClusterBinding tests enforcement with scoped cluster role bindings
func TestCasbinEnforcer_Evaluate_ScopedClusterBinding(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// Setup: cluster roles with specific actions (no wildcards to properly test scope enforcement)
	syncGroupingPolicies(t, enforcer, [][]string{
		{"ns-viewer", "namespace:view", "*"},
		{"ns-viewer", "project:view", "*"},
		{"global-editor", "namespace:view", "*"},
		{"global-editor", "project:view", "*"},
		{"global-editor", "project:update", "*"},
	})

	// Scoped cluster binding: allows ns-viewer only within ns/acme
	// Unscoped cluster binding: allows global-editor everywhere (but with explicit actions, not wildcard)
	syncPolicies(t, enforcer, [][]string{
		{"groups:scoped-team", "ns/acme", "ns-viewer", "*", "allow", "{}", "scoped-crb"},
		{"groups:global-team", "*", "global-editor", "*", "allow", "{}", "global-crb"},
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
			name:              "scoped cluster binding allows within scope",
			entitlementValues: []string{"scoped-team"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1"},
			action:            "project:view",
			want:              true,
			reason:            "scoped binding at ns/acme should allow access to ns/acme/project/p1",
		},
		{
			name:              "scoped cluster binding denies outside scope",
			entitlementValues: []string{"scoped-team"},
			resource:          authzcore.ResourceHierarchy{Namespace: "other", Project: "p1"},
			action:            "project:view",
			want:              false,
			reason:            "scoped binding at ns/acme should not allow access in ns/other",
		},
		{
			name:              "unscoped cluster binding allows within any namespace",
			entitlementValues: []string{"global-team"},
			resource:          authzcore.ResourceHierarchy{Namespace: "anything", Project: "p1"},
			action:            "project:view",
			want:              true,
			reason:            "unscoped binding with * should allow access everywhere",
		},
		{
			name:              "unscoped cluster binding allows action not in scoped role",
			entitlementValues: []string{"global-team"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1"},
			action:            "project:update",
			want:              true,
			reason:            "global-editor has project:update, unscoped binding should allow it",
		},
		{
			name:              "scoped cluster binding denies action not in its role",
			entitlementValues: []string{"scoped-team"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1"},
			action:            "project:update",
			want:              false,
			reason:            "ns-viewer does not have project:update, scoped binding should deny it even within scope",
		},
		{
			name:              "unscoped cluster binding denies action not in its role",
			entitlementValues: []string{"global-team"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1"},
			action:            "project:delete",
			want:              false,
			reason:            "global-editor does not have project:delete, should deny even though binding is unscoped",
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

// =============================================================================
// enforcer.go coverage
// =============================================================================

func TestNewEnforcer_NilClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err := NewEnforcer(context.Background(), CasbinConfig{K8sClient: nil}, logger)
	require.Error(t, err, "NewEnforcer() with nil K8sClient should return error")
}

func TestNewEnforcer_WithCache(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ce := setupTestEnforcer(t) // reuse the fake client from the shared helper
	enforcer, err := NewEnforcer(context.Background(), CasbinConfig{
		K8sClient:    ce.k8sClient,
		CacheEnabled: true,
		CacheTTL:     time.Minute,
	}, logger)
	require.NoError(t, err, "NewEnforcer() with cache error")
	require.True(t, enforcer.enableCache, "NewEnforcer() with CacheEnabled=true should set enableCache=true")
	require.Equal(t, int(time.Minute), enforcer.cacheTTL, "NewEnforcer() cacheTTL mismatch")
}

func TestNewEnforcer_WithCacheDefaultTTL(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ce := setupTestEnforcer(t)
	// CacheTTL=0 should use the default (5m)
	enforcer, err := NewEnforcer(context.Background(), CasbinConfig{
		K8sClient:    ce.k8sClient,
		CacheEnabled: true,
		CacheTTL:     0,
	}, logger)
	require.NoError(t, err, "NewEnforcer() with zero TTL error")
	require.True(t, enforcer.enableCache, "NewEnforcer() with CacheEnabled=true should set enableCache=true")
	require.Equal(t, int(5*time.Minute), enforcer.cacheTTL, "NewEnforcer() cacheTTL should use 5m default")
}

func TestCasbinEnforcer_GetEnforcer(t *testing.T) {
	ce := setupTestEnforcer(t)
	e := ce.GetEnforcer()
	require.NotNil(t, e, "GetEnforcer() returned nil")
}

// =============================================================================
// pdp.go coverage — validation errors, context cancellation, resource ID in reason
// =============================================================================

func TestCasbinEnforcer_Evaluate_NilRequest(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	_, err := enforcer.Evaluate(context.Background(), nil)
	require.Error(t, err, "Evaluate(nil) should return error")
}

func TestCasbinEnforcer_BatchEvaluate_NilRequest(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	_, err := enforcer.BatchEvaluate(context.Background(), nil)
	require.Error(t, err, "BatchEvaluate(nil) should return error")
}

func TestCasbinEnforcer_BatchEvaluate_ContextCancelled(t *testing.T) {
	enforcer := setupTestEnforcer(t)

	// Pre-seed a policy so the first request succeeds
	syncGroupingPolicies(t, enforcer, [][]string{
		{"viewer", "component:view", "*"},
	})
	syncPolicies(t, enforcer, [][]string{
		{"groups:devs", "*", "viewer", "*", "allow", "{}", "binding-ctx"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	req := &authzcore.BatchEvaluateRequest{
		Requests: []authzcore.EvaluateRequest{
			{
				SubjectContext: &authzcore.SubjectContext{
					Type:              "user",
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"devs"},
				},
				Resource: authzcore.Resource{Type: "component"},
				Action:   "component:view",
			},
		},
	}

	_, err := enforcer.BatchEvaluate(ctx, req)
	require.Error(t, err, "BatchEvaluate with cancelled context should return error")
}

func TestCasbinEnforcer_GetSubjectProfile_MissingSubjectContext(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	// Pass a non-nil request with nil SubjectContext; validateProfileRequest will return an error.
	_, err := enforcer.GetSubjectProfile(context.Background(), &authzcore.ProfileRequest{
		SubjectContext: nil,
	})
	require.Error(t, err, "GetSubjectProfile with nil SubjectContext should return error")
}

func TestCasbinEnforcer_Check_NilSubjectContext(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	// Call check directly to exercise the nil subjectCtx guard
	decision, err := enforcer.check(&authzcore.EvaluateRequest{
		SubjectContext: nil,
		Resource:       authzcore.Resource{Type: "component"},
		Action:         "component:view",
	})
	require.Error(t, err, "check with nil SubjectContext should return error")
	require.NotNil(t, decision, "check with nil SubjectContext should return a decision")
	require.False(t, decision.Decision, "check with nil SubjectContext should return deny decision")
}

func TestCasbinEnforcer_FilterPolicies_MalformedPolicy(t *testing.T) {
	enforcer := setupTestEnforcer(t)

	subjectCtx := &authzcore.SubjectContext{
		Type:              "user",
		EntitlementClaim:  "groups",
		EntitlementValues: []string{"devs"},
	}

	// Seed a valid 7-field policy and a malformed 3-field policy via the raw enforcer
	_, _ = enforcer.enforcer.AddPolicy("groups:devs", "ns/acme", "viewer", "acme", "allow", "{}", "b1")
	// Add a malformed policy (only 3 fields) — filterPoliciesBySubjectAndScope must skip it
	_, _ = enforcer.enforcer.AddPolicy("groups:devs", "ns/acme", "bad")

	profile, err := enforcer.GetSubjectProfile(context.Background(), &authzcore.ProfileRequest{
		SubjectContext: subjectCtx,
		Scope:          authzcore.ResourceHierarchy{Namespace: "acme"},
	})
	// The malformed policy must be skipped silently (logged as warning); no error expected
	require.NoError(t, err, "GetSubjectProfile with malformed policy returned unexpected error")
	require.NotNil(t, profile, "GetSubjectProfile returned nil profile")
	// The malformed tuple has roleName "bad"; since it is skipped, no capability
	// derived from "bad" should appear — capabilities may be empty or contain only
	// entries from the valid policy (which itself has no grouping rules seeded here).
	for action, cap := range profile.Capabilities {
		for _, allowed := range cap.Allowed {
			require.NotEqual(t, "bad", allowed.Path, "capability for action %s contains unexpected 'bad' path from malformed policy", action)
		}
	}
}

func TestCasbinEnforcer_Check_WithResourceID(t *testing.T) {
	enforcer := setupTestEnforcer(t)

	// Seed policies: viewer role grants component:view, bound to groups:devs on ns/acme
	syncGroupingPolicies(t, enforcer, [][]string{
		{"viewer", "component:view", "*"},
	})
	syncPolicies(t, enforcer, [][]string{
		{"groups:devs", "ns/acme", "viewer", "*", "allow", "{}", "binding-rid"},
	})

	// Evaluate with a non-empty Resource.ID to exercise the resourceInfo branch
	decision, err := enforcer.Evaluate(context.Background(), &authzcore.EvaluateRequest{
		SubjectContext: &authzcore.SubjectContext{
			Type:              "user",
			EntitlementClaim:  "groups",
			EntitlementValues: []string{"devs"},
		},
		Resource: authzcore.Resource{
			Type: "component",
			ID:   "comp-123", // non-empty ID
			Hierarchy: authzcore.ResourceHierarchy{
				Namespace: "acme",
			},
		},
		Action: "component:view",
	})
	require.NoError(t, err, "Evaluate() error")
	require.True(t, decision.Decision, "Evaluate() expected allow, got deny")
	require.NotNil(t, decision.Context, "Evaluate() expected non-nil context")
	require.NotEmpty(t, decision.Context.Reason, "Evaluate() expected non-empty reason")
}

func TestCasbinEnforcer_BatchEvaluate_CheckError(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	req := &authzcore.BatchEvaluateRequest{
		Requests: []authzcore.EvaluateRequest{
			{
				SubjectContext: &authzcore.SubjectContext{
					Type:              "user",
					EntitlementClaim:  "", // empty claim → formatSubject error in check()
					EntitlementValues: []string{"devs"},
				},
				Resource: authzcore.Resource{Type: "component"},
				Action:   "component:view",
			},
		},
	}
	_, err := enforcer.BatchEvaluate(context.Background(), req)
	require.Error(t, err, "BatchEvaluate with empty EntitlementClaim should return error")
}

func TestCasbinEnforcer_GetSubjectProfile_FormatSubjectError(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	// Empty EntitlementClaim causes formatSubject to error inside filterPoliciesBySubjectAndScope
	_, err := enforcer.GetSubjectProfile(context.Background(), &authzcore.ProfileRequest{
		SubjectContext: &authzcore.SubjectContext{
			Type:              "user",
			EntitlementClaim:  "", // empty claim
			EntitlementValues: []string{"devs"},
		},
		Scope: authzcore.ResourceHierarchy{Namespace: "acme"},
	})
	require.Error(t, err, "GetSubjectProfile with empty EntitlementClaim should return error")
}

func TestCasbinEnforcer_GetSubjectProfile_OutOfScopePolicy(t *testing.T) {
	enforcer := setupTestEnforcer(t)

	// Seed a role and a policy for "ns/other" — unrelated to the queried scope "ns/acme"
	syncGroupingPolicies(t, enforcer, [][]string{
		{"viewer", "component:view", "*"},
	})
	syncPolicies(t, enforcer, [][]string{
		// Policy for "ns/other" — completely outside scope "ns/acme"
		{"groups:devs", "ns/other", "viewer", "*", "allow", "{}", "oos-binding"},
	})

	result, err := enforcer.GetSubjectProfile(context.Background(), &authzcore.ProfileRequest{
		SubjectContext: &authzcore.SubjectContext{
			Type:              "user",
			EntitlementClaim:  "groups",
			EntitlementValues: []string{"devs"},
		},
		Scope: authzcore.ResourceHierarchy{Namespace: "acme"}, // "ns/acme" — out-of-scope policy is skipped
	})
	require.NoError(t, err, "GetSubjectProfile out-of-scope error")
	require.NotNil(t, result, "GetSubjectProfile returned nil result")
	// The out-of-scope policy (ns/other) must not appear in capabilities for scope ns/acme
	require.Empty(t, result.Capabilities, "expected no capabilities for out-of-scope policy")
}
