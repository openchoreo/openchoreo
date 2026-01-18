// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

const (
	testRoleName         = "test-role"
	testEntitlementType  = "group"
	testEntitlementValue = "test-group"
	user                 = "user"
)

// setupTestEnforcer creates a test CasbinEnforcer with temporary database
func setupTestEnforcer(t *testing.T) *CasbinEnforcer {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create enforcer with default user type configs
	config := CasbinConfig{
		DatabasePath: dbPath,
		EnableCache:  false,
	}

	enforcer, err := NewCasbinEnforcer(config, logger)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}

	t.Cleanup(func() {
		if err := enforcer.Close(); err != nil {
			t.Errorf("failed to close enforcer: %v", err)
		}
	})

	return enforcer
}

// TestCasbinEnforcer_Evaluate_ClusterRoles_Focused tests authorization with cluster-scoped roles only
func TestCasbinEnforcer_Evaluate_ClusterRoles_Focused(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// Setup: Create cluster role with multiple actions and wildcards
	multiRole := &authzcore.Role{
		Name:    "multi-role",
		Actions: []string{"organization:view", "component:*", "project:view"},
	}
	if err := enforcer.AddRole(ctx, multiRole); err != nil {
		t.Fatalf("failed to add multi-role: %v", err)
	}
	multiRoleMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "test-group",
		},
		RoleRef: authzcore.RoleRef{Name: "multi-role"},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, multiRoleMapping); err != nil {
		t.Fatalf("failed to add multi-role mapping: %v", err)
	}

	// Setup: Global wildcard role
	globalRole := &authzcore.Role{
		Name:    "global-admin",
		Actions: []string{"*"},
	}
	if err := enforcer.AddRole(ctx, globalRole); err != nil {
		t.Fatalf("failed to add global-admin role: %v", err)
	}
	globalMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "global-admin-group",
		},
		RoleRef:   authzcore.RoleRef{Name: "global-admin"},
		Hierarchy: authzcore.ResourceHierarchy{},
		Effect:    authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, globalMapping); err != nil {
		t.Fatalf("failed to add global mapping: %v", err)
	}

	// Setup: Component-level scoped role
	componentRole := &authzcore.Role{
		Name:    "component-deployer",
		Actions: []string{"component:deploy"},
	}
	if err := enforcer.AddRole(ctx, componentRole); err != nil {
		t.Fatalf("failed to add component-deployer role: %v", err)
	}
	componentMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "component-group",
		},
		RoleRef: authzcore.RoleRef{Name: "component-deployer"},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
			Project:   "p1",
			Component: "c1",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, componentMapping); err != nil {
		t.Fatalf("failed to add component mapping: %v", err)
	}

	// Setup: Project-level scoped role
	projectRole := &authzcore.Role{
		Name:    "project-admin",
		Actions: []string{"project:*", "component:create"},
	}
	if err := enforcer.AddRole(ctx, projectRole); err != nil {
		t.Fatalf("failed to add project-admin role: %v", err)
	}
	projectMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "project-group",
		},
		RoleRef: authzcore.RoleRef{Name: "project-admin"},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
			Project:   "p2",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, projectMapping); err != nil {
		t.Fatalf("failed to add project mapping: %v", err)
	}

	// Setup: Multiple roles for same user
	readerRole := &authzcore.Role{
		Name:    "reader",
		Actions: []string{"component:view", "project:view"},
	}
	writerRole := &authzcore.Role{
		Name:    "writer",
		Actions: []string{"component:create", "project:create"},
	}
	if err := enforcer.AddRole(ctx, readerRole); err != nil {
		t.Fatalf("failed to add reader role: %v", err)
	}
	if err := enforcer.AddRole(ctx, writerRole); err != nil {
		t.Fatalf("failed to add writer role: %v", err)
	}
	for _, roleName := range []string{"reader", "writer"} {
		mapping := &authzcore.RoleEntitlementMapping{
			Entitlement: authzcore.Entitlement{
				Claim: "groups",
				Value: "multi-role-group",
			},
			RoleRef:   authzcore.RoleRef{Name: roleName},
			Hierarchy: authzcore.ResourceHierarchy{Namespace: "acme"},
			Effect:    authzcore.PolicyEffectAllow,
		}
		if err := enforcer.AddRoleEntitlementMapping(ctx, mapping); err != nil {
			t.Fatalf("failed to add %s mapping: %v", roleName, err)
		}
	}

	// Setup: Role for 'sub' claim testing
	subClaimRole := &authzcore.Role{
		Name:    "sub-claim-role",
		Actions: []string{"component:view"},
	}
	if err := enforcer.AddRole(ctx, subClaimRole); err != nil {
		t.Fatalf("failed to add sub-claim-role: %v", err)
	}
	subClaimMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "sub",
			Value: "user-123",
		},
		RoleRef:   authzcore.RoleRef{Name: "sub-claim-role"},
		Hierarchy: authzcore.ResourceHierarchy{Namespace: "acme"},
		Effect:    authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, subClaimMapping); err != nil {
		t.Fatalf("failed to add sub claim mapping: %v", err)
	}

	// Setup: Service account role
	serviceRole := &authzcore.Role{
		Name:    "service-account-role",
		Actions: []string{"component:deploy", "component:view"},
	}
	if err := enforcer.AddRole(ctx, serviceRole); err != nil {
		t.Fatalf("failed to add service-account-role: %v", err)
	}
	serviceSAMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "sa-group",
		},
		RoleRef:   authzcore.RoleRef{Name: "service-account-role"},
		Hierarchy: authzcore.ResourceHierarchy{Namespace: "acme"},
		Effect:    authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, serviceSAMapping); err != nil {
		t.Fatalf("failed to add service account mapping: %v", err)
	}

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
			action:            "organization:view",
			want:              true,
			reason:            "organization:* should match organization:view",
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
			resource:          authzcore.ResourceHierarchy{Namespace: "other-org", Project: "p1"},
			action:            "component:view",
			want:              false,
			reason:            "policy scoped to 'acme', not 'other-org'",
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
			resource:          authzcore.ResourceHierarchy{Namespace: "any-org", Project: "any-project"},
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
			action:            "organization:view",
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

	// Setup: Create namespace role in 'acme'
	nsEngineerRole := &authzcore.Role{
		Name:      "ns-engineer",
		Namespace: "acme",
		Actions:   []string{"component:deploy", "component:view", "project:view"},
	}
	if err := enforcer.AddRole(ctx, nsEngineerRole); err != nil {
		t.Fatalf("failed to add ns-engineer role: %v", err)
	}
	nsEngineerMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "ns-engineer-group",
		},
		RoleRef: authzcore.RoleRef{Name: "ns-engineer", Namespace: "acme"},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, nsEngineerMapping); err != nil {
		t.Fatalf("failed to add ns-engineer mapping: %v", err)
	}

	// Setup: Namespace role with project-level scope
	nsProjectLeadRole := &authzcore.Role{
		Name:      "ns-project-lead",
		Namespace: "acme",
		Actions:   []string{"project:*", "component:*"},
	}
	if err := enforcer.AddRole(ctx, nsProjectLeadRole); err != nil {
		t.Fatalf("failed to add ns-project-lead role: %v", err)
	}
	nsProjectLeadMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "ns-project-lead-group",
		},
		RoleRef: authzcore.RoleRef{Name: "ns-project-lead", Namespace: "acme"},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
			Project:   "p1",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, nsProjectLeadMapping); err != nil {
		t.Fatalf("failed to add ns-project-lead mapping: %v", err)
	}

	// Setup: Multiple namespace roles for same user
	nsReaderRole := &authzcore.Role{
		Name:      "ns-reader",
		Namespace: "acme",
		Actions:   []string{"component:view", "project:view"},
	}
	nsWriterRole := &authzcore.Role{
		Name:      "ns-writer",
		Namespace: "acme",
		Actions:   []string{"component:create", "project:create"},
	}
	if err := enforcer.AddRole(ctx, nsReaderRole); err != nil {
		t.Fatalf("failed to add ns-reader role: %v", err)
	}
	if err := enforcer.AddRole(ctx, nsWriterRole); err != nil {
		t.Fatalf("failed to add ns-writer role: %v", err)
	}
	for _, roleName := range []string{"ns-reader", "ns-writer"} {
		mapping := &authzcore.RoleEntitlementMapping{
			Entitlement: authzcore.Entitlement{
				Claim: "groups",
				Value: "ns-multi-role-group",
			},
			RoleRef:   authzcore.RoleRef{Name: roleName, Namespace: "acme"},
			Hierarchy: authzcore.ResourceHierarchy{Namespace: "acme"},
			Effect:    authzcore.PolicyEffectAllow,
		}
		if err := enforcer.AddRoleEntitlementMapping(ctx, mapping); err != nil {
			t.Fatalf("failed to add %s mapping: %v", roleName, err)
		}
	}

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

	// Setup: Cluster role for deny scenarios
	clusterRole := &authzcore.Role{
		Name:    "developer",
		Actions: []string{"component:*", "project:view"},
	}
	if err := enforcer.AddRole(ctx, clusterRole); err != nil {
		t.Fatalf("failed to add cluster developer role: %v", err)
	}

	// Allow policy at namespace level
	clusterAllowMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{Claim: "groups", Value: "user-group"},
		RoleRef:     authzcore.RoleRef{Name: "developer"},
		Hierarchy:   authzcore.ResourceHierarchy{Namespace: "acme"},
		Effect:      authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, clusterAllowMapping); err != nil {
		t.Fatalf("failed to add cluster allow mapping: %v", err)
	}

	// Deny policy at project level
	clusterDenyMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{Claim: "groups", Value: "user-group"},
		RoleRef:     authzcore.RoleRef{Name: "developer"},
		Hierarchy:   authzcore.ResourceHierarchy{Namespace: "acme", Project: "secret"},
		Effect:      authzcore.PolicyEffectDeny,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, clusterDenyMapping); err != nil {
		t.Fatalf("failed to add cluster deny mapping: %v", err)
	}

	// Setup: Namespace role for deny scenarios
	nsRole := &authzcore.Role{
		Name:      "ns-developer",
		Namespace: "acme",
		Actions:   []string{"component:*", "project:*"},
	}
	if err := enforcer.AddRole(ctx, nsRole); err != nil {
		t.Fatalf("failed to add ns-developer role: %v", err)
	}

	// Namespace role allow at namespace level
	nsAllowMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{Claim: "groups", Value: "ns-user-group"},
		RoleRef:     authzcore.RoleRef{Name: "ns-developer", Namespace: "acme"},
		Hierarchy:   authzcore.ResourceHierarchy{Namespace: "acme"},
		Effect:      authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, nsAllowMapping); err != nil {
		t.Fatalf("failed to add namespace allow mapping: %v", err)
	}

	// Namespace role deny at component level
	nsDenyMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{Claim: "groups", Value: "ns-user-group"},
		RoleRef:     authzcore.RoleRef{Name: "ns-developer", Namespace: "acme"},
		Hierarchy:   authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "restricted"},
		Effect:      authzcore.PolicyEffectDeny,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, nsDenyMapping); err != nil {
		t.Fatalf("failed to add namespace deny mapping: %v", err)
	}

	// Cross-role-type deny: namespace role deny overrides cluster role allow
	mixedDenyMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{Claim: "groups", Value: "user-group"},
		RoleRef:     authzcore.RoleRef{Name: "ns-developer", Namespace: "acme"},
		Hierarchy:   authzcore.ResourceHierarchy{Namespace: "acme", Project: "public", Component: "forbidden"},
		Effect:      authzcore.PolicyEffectDeny,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, mixedDenyMapping); err != nil {
		t.Fatalf("failed to add mixed deny mapping: %v", err)
	}

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

	// Setup: Namespace role in 'acme'
	nsRoleAcme := &authzcore.Role{
		Name:      "ns-engineer",
		Namespace: "acme",
		Actions:   []string{"component:deploy", "component:view", "project:view"},
	}
	if err := enforcer.AddRole(ctx, nsRoleAcme); err != nil {
		t.Fatalf("failed to add ns-engineer role for acme: %v", err)
	}
	nsMappingAcme := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "acme-engineer-group",
		},
		RoleRef: authzcore.RoleRef{Name: "ns-engineer", Namespace: "acme"},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, nsMappingAcme); err != nil {
		t.Fatalf("failed to add ns-engineer mapping for acme: %v", err)
	}

	// Setup: Namespace role with SAME NAME in 'other-org'
	nsRoleOther := &authzcore.Role{
		Name:      "ns-engineer",
		Namespace: "other-org",
		Actions:   []string{"component:delete", "project:delete"},
	}
	if err := enforcer.AddRole(ctx, nsRoleOther); err != nil {
		t.Fatalf("failed to add ns-engineer role for other-org: %v", err)
	}
	nsMappingOther := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "other-engineer-group",
		},
		RoleRef: authzcore.RoleRef{Name: "ns-engineer", Namespace: "other-org"},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "other-org",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, nsMappingOther); err != nil {
		t.Fatalf("failed to add ns-engineer mapping for other-org: %v", err)
	}

	tests := []struct {
		name              string
		entitlementValues []string
		resource          authzcore.ResourceHierarchy
		action            string
		want              bool
		reason            string
	}{
		// Test acme role works in acme
		{
			name:              "acme role - works in own namespace",
			entitlementValues: []string{"acme-engineer-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:deploy",
			want:              true,
			reason:            "namespace role should work in its own namespace",
		},
		// Test acme role CANNOT access other-org
		{
			name:              "acme role - no access to other namespace",
			entitlementValues: []string{"acme-engineer-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "other-org", Project: "p1", Component: "c1"},
			action:            "component:deploy",
			want:              false,
			reason:            "namespace role for 'acme' should NOT grant access to 'other-org'",
		},
		{
			name:              "other-org role - works in own namespace",
			entitlementValues: []string{"other-engineer-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "other-org", Project: "p1", Component: "c1"},
			action:            "component:delete",
			want:              true,
			reason:            "namespace role should work in its own namespace",
		},
		// Test other-org role CANNOT access acme
		{
			name:              "other-org role - no access to acme namespace",
			entitlementValues: []string{"other-engineer-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1"},
			action:            "project:delete",
			want:              false,
			reason:            "namespace role for 'other-org' should NOT grant access to 'acme'",
		},
		// Test roles with same name are independent
		{
			name:              "same role name - acme permissions don't leak to other-org",
			entitlementValues: []string{"acme-engineer-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "acme", Project: "p1", Component: "c1"},
			action:            "component:delete",
			want:              false,
			reason:            "acme ns-engineer role doesn't have delete (only other-org does)",
		},
		{
			name:              "same role name - other-org permissions don't leak to acme",
			entitlementValues: []string{"other-engineer-group"},
			resource:          authzcore.ResourceHierarchy{Namespace: "other-org", Project: "p1", Component: "c1"},
			action:            "component:deploy",
			want:              false,
			reason:            "other-org ns-engineer role doesn't have deploy (only acme does)",
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

	// Setup: Create cluster role
	clusterRole := &authzcore.Role{
		Name:    "developer",
		Actions: []string{"component:view", "project:view"},
	}
	if err := enforcer.AddRole(ctx, clusterRole); err != nil {
		t.Fatalf("failed to add cluster-developer role: %v", err)
	}

	// Setup: Create namespace role with same actions but different name
	namespaceRole := &authzcore.Role{
		Name:      "developer",
		Namespace: "acme",
		Actions:   []string{"component:deploy", "component:view", "project:view"},
	}
	if err := enforcer.AddRole(ctx, namespaceRole); err != nil {
		t.Fatalf("failed to add ns-developer role: %v", err)
	}

	// Setup: Add cluster role mapping at namespace level
	clusterMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "cluster-users",
		},
		RoleRef: authzcore.RoleRef{Name: "developer"},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, clusterMapping); err != nil {
		t.Fatalf("failed to add cluster role mapping: %v", err)
	}

	// Setup: Add namespace role mapping
	nsMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "ns-users",
		},
		RoleRef: authzcore.RoleRef{Name: "developer", Namespace: "acme"},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, nsMapping); err != nil {
		t.Fatalf("failed to add namespace role mapping: %v", err)
	}

	// Same group with BOTH cluster role AND namespace role
	clusterViewerRole := &authzcore.Role{
		Name:    "viewer",
		Actions: []string{"component:view", "project:view"},
	}
	if err := enforcer.AddRole(ctx, clusterViewerRole); err != nil {
		t.Fatalf("failed to add viewer cluster role: %v", err)
	}

	nsDeployerRole := &authzcore.Role{
		Name:      "deployer",
		Namespace: "acme",
		Actions:   []string{"component:deploy"},
	}
	if err := enforcer.AddRole(ctx, nsDeployerRole); err != nil {
		t.Fatalf("failed to add deployer namespace role: %v", err)
	}

	// Map BOTH roles to the SAME group "engineering"
	clusterViewerMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "engineering", // Same group
		},
		RoleRef: authzcore.RoleRef{Name: "viewer"}, // Cluster role
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, clusterViewerMapping); err != nil {
		t.Fatalf("failed to add cluster viewer mapping: %v", err)
	}

	nsDeployerMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "engineering", // Same group as cluster role above
		},
		RoleRef: authzcore.RoleRef{Name: "deployer", Namespace: "acme"},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, nsDeployerMapping); err != nil {
		t.Fatalf("failed to add namespace deployer mapping: %v", err)
	}

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
				Namespace: "other-org",
				Project:   "p1",
				Component: "c1",
			},
			action: "component:deploy",
			want:   false,
			reason: "namespace 'deployer' role limited to 'acme', should NOT work in 'other-org'",
		},
		{
			name:              "same group both roles - cluster role should work in other-org",
			entitlementValues: []string{"engineering"},
			resource: authzcore.ResourceHierarchy{
				Namespace: "other-org",
				Project:   "p1",
			},
			action: "project:view",
			want:   false,
			reason: "cluster role mapped only to 'acme' namespace, not 'other-org'",
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

	role1 := &authzcore.Role{
		Name:    "reader",
		Actions: []string{"component:view"},
	}
	role2 := &authzcore.Role{
		Name:    "writer",
		Actions: []string{"component:create"},
	}

	if err := enforcer.AddRole(ctx, role1); err != nil {
		t.Fatalf("failed to add role1: %v", err)
	}
	if err := enforcer.AddRole(ctx, role2); err != nil {
		t.Fatalf("failed to add role2: %v", err)
	}

	mapping1 := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "dev-group",
		},
		RoleRef: authzcore.RoleRef{Name: "reader"},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	mapping2 := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "dev-group",
		},
		RoleRef: authzcore.RoleRef{Name: "writer"},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
			Project:   "p1",
		},
		Effect: authzcore.PolicyEffectAllow,
	}

	if err := enforcer.AddRoleEntitlementMapping(ctx, mapping1); err != nil {
		t.Fatalf("failed to add mapping1: %v", err)
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, mapping2); err != nil {
		t.Fatalf("failed to add mapping2: %v", err)
	}

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
	ctx := context.Background()

	// Setup: Create cluster role
	clusterRole := &authzcore.Role{
		Name:      "viewer",
		Namespace: "", // cluster role
		Actions:   []string{"component:view"},
	}
	if err := enforcer.AddRole(ctx, clusterRole); err != nil {
		t.Fatalf("AddRole(cluster) error = %v", err)
	}

	// Setup: Create namespace-scoped role
	nsRole := &authzcore.Role{
		Name:      "editor",
		Namespace: "acme", // namespace role
		Actions:   []string{"component:edit"},
	}
	if err := enforcer.AddRole(ctx, nsRole); err != nil {
		t.Fatalf("AddRole(namespace) error = %v", err)
	}

	mapping1 := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "group",
			Value: "group1",
		},
		RoleRef: authzcore.RoleRef{Name: "viewer", Namespace: ""},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	mapping2 := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "group",
			Value: "group1",
		},
		RoleRef: authzcore.RoleRef{Name: "viewer", Namespace: ""},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
			Project:   "p1",
		},
		Effect: authzcore.PolicyEffectDeny,
	}
	mapping3 := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "group",
			Value: "group1",
		},
		RoleRef: authzcore.RoleRef{Name: "viewer", Namespace: ""},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "other-org",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	// Namespace-scoped role mapping
	mapping4 := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "group",
			Value: "group1",
		},
		RoleRef: authzcore.RoleRef{Name: "editor", Namespace: "acme"},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}

	if err := enforcer.AddRoleEntitlementMapping(ctx, mapping1); err != nil {
		t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, mapping2); err != nil {
		t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, mapping3); err != nil {
		t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, mapping4); err != nil {
		t.Fatalf("AddRoleEntitlementMapping(ns) error = %v", err)
	}

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
	ctx := context.Background()

	// Setup: Create roles with different actions
	viewerRole := &authzcore.Role{
		Name:    "viewer",
		Actions: []string{"component:view", "project:view"},
	}
	editorRole := &authzcore.Role{
		Name:    "editor",
		Actions: []string{"component:*"},
	}
	adminRole := &authzcore.Role{
		Name:    "admin",
		Actions: []string{"*"},
	}
	nsEditorRole := &authzcore.Role{
		Name:      "editor",
		Namespace: "acme",
		Actions:   []string{"component:view"},
	}

	if err := enforcer.AddRole(ctx, viewerRole); err != nil {
		t.Fatalf("AddRole(viewer) error = %v", err)
	}
	if err := enforcer.AddRole(ctx, editorRole); err != nil {
		t.Fatalf("AddRole(editor) error = %v", err)
	}
	if err := enforcer.AddRole(ctx, adminRole); err != nil {
		t.Fatalf("AddRole(admin) error = %v", err)
	}
	if err := enforcer.AddRole(ctx, nsEditorRole); err != nil {
		t.Fatalf("AddRole(nsEditor) error = %v", err)
	}

	// Create action index for testing
	testActions := []Action{
		{Action: "component:view"},
		{Action: "component:create"},
		{Action: "component:update"},
		{Action: "component:delete"},
		{Action: "project:view"},
		{Action: "project:create"},
		{Action: "organization:view"},
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
				"component:view":    {allowedCount: 1, deniedCount: 0},
				"component:create":  {allowedCount: 1, deniedCount: 0},
				"component:update":  {allowedCount: 1, deniedCount: 0},
				"component:delete":  {allowedCount: 1, deniedCount: 0},
				"project:view":      {allowedCount: 1, deniedCount: 0},
				"project:create":    {allowedCount: 1, deniedCount: 0},
				"organization:view": {allowedCount: 1, deniedCount: 0},
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

// ============================================================================
// PAP Tests - Policy Mapping Management
// ============================================================================

func TestComputeActionsDiff(t *testing.T) {
	tests := []struct {
		name            string
		existingActions []string
		newActions      []string
		wantAdded       []string
		wantRemoved     []string
	}{
		{
			name:            "completely different action sets",
			existingActions: []string{"component:view", "component:create"},
			newActions:      []string{"project:view", "project:create"},
			wantAdded:       []string{"project:view", "project:create"},
			wantRemoved:     []string{"component:view", "component:create"},
		},
		{
			name:            "identical action sets",
			existingActions: []string{"component:view", "component:create"},
			newActions:      []string{"component:view", "component:create"},
			wantAdded:       []string{},
			wantRemoved:     []string{},
		},
		{
			name:            "only additions",
			existingActions: []string{"component:view"},
			newActions:      []string{"component:view", "component:create", "component:delete"},
			wantAdded:       []string{"component:create", "component:delete"},
			wantRemoved:     []string{},
		},
		{
			name:            "only removals",
			existingActions: []string{"component:view", "component:create", "component:delete"},
			newActions:      []string{"component:view"},
			wantAdded:       []string{},
			wantRemoved:     []string{"component:create", "component:delete"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added, removed := computeActionsDiff(tt.existingActions, tt.newActions)

			// Convert to maps for comparison (order doesn't matter)
			addedMap := make(map[string]bool)
			for _, a := range added {
				addedMap[a] = true
			}
			removedMap := make(map[string]bool)
			for _, r := range removed {
				removedMap[r] = true
			}

			wantAddedMap := make(map[string]bool)
			for _, a := range tt.wantAdded {
				wantAddedMap[a] = true
			}
			wantRemovedMap := make(map[string]bool)
			for _, r := range tt.wantRemoved {
				wantRemovedMap[r] = true
			}

			if len(addedMap) != len(wantAddedMap) {
				t.Errorf("computeActionsDiff() added count = %d, want %d", len(addedMap), len(wantAddedMap))
			}
			for action := range wantAddedMap {
				if !addedMap[action] {
					t.Errorf("computeActionsDiff() missing added action: %s", action)
				}
			}

			if len(removedMap) != len(wantRemovedMap) {
				t.Errorf("computeActionsDiff() removed count = %d, want %d", len(removedMap), len(wantRemovedMap))
			}
			for action := range wantRemovedMap {
				if !removedMap[action] {
					t.Errorf("computeActionsDiff() missing removed action: %s", action)
				}
			}
		})
	}
}

// TestCasbinEnforcer_ListRoles_NamespacedRoles tests listing namespace-scoped roles

func TestCasbinEnforcer_GetSubjectProfile(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// Cluster-scoped roles
	viewerRole := &authzcore.Role{
		Name:      "viewer",
		Namespace: "",
		Actions:   []string{"component:view", "project:view"},
	}
	editorRole := &authzcore.Role{
		Name:      "editor",
		Namespace: "",
		Actions:   []string{"component:view", "component:create", "component:update"},
	}
	// Namespace-scoped role
	nsEditorRole := &authzcore.Role{
		Name:      "editor",
		Namespace: "acme",
		Actions:   []string{"project:delete"},
	}

	if err := enforcer.AddRole(ctx, viewerRole); err != nil {
		t.Fatalf("AddRole(viewer) error = %v", err)
	}
	if err := enforcer.AddRole(ctx, editorRole); err != nil {
		t.Fatalf("AddRole(editor) error = %v", err)
	}
	if err := enforcer.AddRole(ctx, nsEditorRole); err != nil {
		t.Fatalf("AddRole(nsEditor) error = %v", err)
	}

	// Setup: Add role entitlement mappings
	clusterEditorMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "dev-group",
		},
		RoleRef: authzcore.RoleRef{Name: "editor", Namespace: ""},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	viewerMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "dev-group",
		},
		RoleRef: authzcore.RoleRef{Name: "viewer", Namespace: ""},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
			Project:   "p1",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	denyMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "dev-group",
		},
		RoleRef: authzcore.RoleRef{Name: "editor", Namespace: ""},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
			Project:   "secret",
		},
		Effect: authzcore.PolicyEffectDeny,
	}
	nsEditorMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "dev-group",
		},
		RoleRef: authzcore.RoleRef{Name: "editor", Namespace: "acme"},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}

	if err := enforcer.AddRoleEntitlementMapping(ctx, clusterEditorMapping); err != nil {
		t.Fatalf("AddRoleEntitlementMapping(clusterEditor) error = %v", err)
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, viewerMapping); err != nil {
		t.Fatalf("AddRoleEntitlementMapping(viewer) error = %v", err)
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, denyMapping); err != nil {
		t.Fatalf("AddRoleEntitlementMapping(deny) error = %v", err)
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, nsEditorMapping); err != nil {
		t.Fatalf("AddRoleEntitlementMapping(nsEditor) error = %v", err)
	}
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

		// Verify role exists in Casbin g policies (cluster roles)
		gPolicies, err := enforcer.enforcer.GetNamedGroupingPolicy("g")
		if err != nil {
			t.Fatalf("GetNamedGroupingPolicy() error = %v", err)
		}
		found := false
		for _, policy := range gPolicies {
			if len(policy) >= 3 && policy[0] == testRoleName && policy[2] == "*" {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("role %s not found in g policies", testRoleName)
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

		// Add role second time - should fail
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

		// Verify role exists in Casbin g policies (unified grouping with namespace)
		gPolicies, err := enforcer.enforcer.GetNamedGroupingPolicy("g")
		if err != nil {
			t.Fatalf("GetNamedGroupingPolicy() error = %v", err)
		}
		found := false
		for _, policy := range gPolicies {
			if len(policy) >= 3 && policy[0] == "ns-developer" && policy[2] == testNs {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("namespace role 'ns-developer' in namespace '%s' not found in g policies", testNs)
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

		// Verify both cluster and namespace roles exist in g policies
		// Format: g, <role>, <action>, <namespace>
		// Cluster roles have namespace = "*", namespace roles have namespace = "acme"
		gPolicies, err := enforcer.enforcer.GetNamedGroupingPolicy("g")
		if err != nil {
			t.Fatalf("GetNamedGroupingPolicy(g) error = %v", err)
		}
		foundCluster := false
		foundNs := false
		for _, policy := range gPolicies {
			if len(policy) >= 3 && policy[0] == "admin" {
				if policy[2] == "*" {
					foundCluster = true
				}
				if policy[2] == testNs {
					foundNs = true
				}
			}
		}
		if !foundCluster {
			t.Error("cluster role 'admin' not found in g policies")
		}
		if !foundNs {
			t.Errorf("namespace role 'admin' in namespace '%s' not found in g policies", testNs)
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

		// Verify both exist in g policies
		// Format: g, <role>, <action>, <namespace>
		gPolicies, err := enforcer.enforcer.GetNamedGroupingPolicy("g")
		if err != nil {
			t.Fatalf("GetNamedGroupingPolicy(g) error = %v", err)
		}

		foundAcme := false
		foundWidgets := false
		for _, policy := range gPolicies {
			if len(policy) >= 3 && policy[0] == "engineer" {
				if policy[2] == testNs {
					foundAcme = true
				}
				if policy[2] == "widgets" {
					foundWidgets = true
				}
			}
		}

		if !foundAcme {
			t.Errorf("namespace role 'engineer' in namespace '%s' not found in g policies", testNs)
		}
		if !foundWidgets {
			t.Error("namespace role 'engineer' in namespace 'widgets' not found in g policies")
		}
	})
}

// TestCasbinEnforcer_GetRole tests retrieving cluster-scoped roles
func TestCasbinEnforcer_GetRole(t *testing.T) {
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
	// Using "acme" for namespace-scoped role``
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

// TestCasbinEnforcer_RemoveRole tests removing cluster-scoped roles
func TestCasbinEnforcer_RemoveRole_ClusterRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("success - remove role with no mappings", func(t *testing.T) {
		// Setup: Add role directly using Casbin grouping policy
		_, err := enforcer.enforcer.AddGroupingPolicy("removable-role", "component:view", "*")
		if err != nil {
			t.Fatalf("AddGroupingPolicy() error = %v", err)
		}

		err = enforcer.RemoveRole(ctx, &authzcore.RoleRef{Name: "removable-role"})
		if err != nil {
			t.Fatalf("RemoveRole() error = %v", err)
		}

		gPolicies, err := enforcer.enforcer.GetNamedGroupingPolicy("g")
		if err != nil {
			t.Fatalf("GetNamedGroupingPolicy() error = %v", err)
		}
		for _, policy := range gPolicies {
			if len(policy) >= 2 && policy[0] == "removable-role" {
				t.Error("RemoveRole() role still exists in g policies")
			}
		}
	})

	t.Run("non-existent role", func(t *testing.T) {
		err := enforcer.RemoveRole(ctx, &authzcore.RoleRef{Name: "non-existent-role"})
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("RemoveRole() error = %v, want ErrRoleNotFound", err)
		}
	})

	t.Run("role in use", func(t *testing.T) {
		roleRules := [][]string{
			{"in-use-role", "component:view", "*"},
			{"in-use-role", "component:create", "*"},
		}
		_, err := enforcer.enforcer.AddGroupingPolicies(roleRules)
		if err != nil {
			t.Fatalf("AddGroupingPolicies() error = %v", err)
		}

		_, err = enforcer.enforcer.AddPolicy("groups:test-group", "orgs/acme", "in-use-role", "*", "allow", "{}")
		if err != nil {
			t.Fatalf("AddPolicy() error = %v", err)
		}

		err = enforcer.RemoveRole(ctx, &authzcore.RoleRef{Name: "in-use-role"})
		if !errors.Is(err, authzcore.ErrRoleInUse) {
			t.Errorf("RemoveRole() error = %v, want ErrRoleInUse", err)
		}
	})
}

// TestCasbinEnforcer_RemoveRole_NamespacedRole tests removing namespace-scoped roles
func TestCasbinEnforcer_RemoveRole_NamespacedRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()
	const testNs = "acme"

	t.Run("remove namespace role not in use", func(t *testing.T) {
		_, err := enforcer.enforcer.AddGroupingPolicy("unused-role", "component:view", testNs)
		if err != nil {
			t.Fatalf("failed to add namespace role via grouping policy: %v", err)
		}

		if err := enforcer.RemoveRole(ctx, &authzcore.RoleRef{Name: "unused-role", Namespace: testNs}); err != nil {
			t.Fatalf("failed to remove namespace role: %v", err)
		}

		// Verify role is removed from g policies
		gPolicies, err := enforcer.enforcer.GetNamedGroupingPolicy("g")
		if err != nil {
			t.Fatalf("GetNamedGroupingPolicy() error = %v", err)
		}
		for _, policy := range gPolicies {
			if len(policy) >= 3 && policy[0] == "unused-role" && policy[2] == testNs {
				t.Error("RemoveRole() namespace role still exists in g policies")
			}
		}
	})

	t.Run("remove namespace role in use fails", func(t *testing.T) {
		_, err := enforcer.enforcer.AddGroupingPolicy("in-use-role", "component:view", testNs)
		if err != nil {
			t.Fatalf("failed to add namespace role via grouping policy: %v", err)
		}

		_, err = enforcer.enforcer.AddPolicy("groups:test-group", "orgs/acme", "in-use-role", testNs, "allow", "{}")
		if err != nil {
			t.Fatalf("failed to add mapping: %v", err)
		}

		err = enforcer.RemoveRole(ctx, &authzcore.RoleRef{Name: "in-use-role", Namespace: testNs})
		if !errors.Is(err, authzcore.ErrRoleInUse) {
			t.Errorf("expected ErrRoleInUse, got %v", err)
		}
	})
}

// TestCasbinEnforcer_ForceRemoveRole_ClusterRole tests force removing cluster-scoped roles
func TestCasbinEnforcer_ForceRemoveRole_ClusterRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("force remove role with associated mappings", func(t *testing.T) {
		_, err := enforcer.enforcer.AddGroupingPolicy("force-removable", "component:view", "*")
		if err != nil {
			t.Fatalf("AddGroupingPolicy() error = %v", err)
		}

		_, err = enforcer.enforcer.AddPolicy("groups:test-group", "orgs/acme", "force-removable", "*", "allow", "{}")
		if err != nil {
			t.Fatalf("AddPolicy() error = %v", err)
		}

		err = enforcer.ForceRemoveRole(ctx, &authzcore.RoleRef{Name: "force-removable"})
		if err != nil {
			t.Fatalf("ForceRemoveRole() error = %v", err)
		}

		gPolicies, err := enforcer.enforcer.GetNamedGroupingPolicy("g")
		if err != nil {
			t.Fatalf("GetNamedGroupingPolicy() error = %v", err)
		}
		for _, policy := range gPolicies {
			if len(policy) >= 2 && policy[0] == "force-removable" {
				t.Error("ForceRemoveRole() role still exists in g policies")
			}
		}

		policies, err := enforcer.enforcer.GetPolicy()
		if err != nil {
			t.Fatalf("GetPolicy() error = %v", err)
		}
		mappingCount := 0
		for _, policy := range policies {
			if len(policy) >= 3 && policy[2] == "force-removable" {
				mappingCount++
			}
		}
		if mappingCount != 0 {
			t.Errorf("ForceRemoveRole() expected 0 mappings, got %d", mappingCount)
		}
	})

	t.Run("force remove non-existent role", func(t *testing.T) {
		err := enforcer.ForceRemoveRole(ctx, &authzcore.RoleRef{Name: "non-existent"})
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("ForceRemoveRole() error = %v, want ErrRoleNotFound", err)
		}
	})

	t.Run("force remove role without mappings", func(t *testing.T) {
		_, err := enforcer.enforcer.AddGroupingPolicy("no-mappings-role", "component:view", "*")
		if err != nil {
			t.Fatalf("AddGroupingPolicy() error = %v", err)
		}

		err = enforcer.ForceRemoveRole(ctx, &authzcore.RoleRef{Name: "no-mappings-role"})
		if err != nil {
			t.Fatalf("ForceRemoveRole() error = %v", err)
		}

		_, err = enforcer.GetRole(ctx, &authzcore.RoleRef{Name: "no-mappings-role"})
		if err == nil {
			t.Error("ForceRemoveRole() role still exists after removal")
		}
	})
}

// TestCasbinEnforcer_ForceRemoveRole_NamespacedRole tests force removing namespace-scoped roles
func TestCasbinEnforcer_ForceRemoveRole_NamespacedRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	const testNs = "acme"

	_, err := enforcer.enforcer.AddGroupingPolicy("ns-admin", "*", testNs)
	if err != nil {
		t.Fatalf("failed to add namespace role via grouping policy: %v", err)
	}

	_, err = enforcer.enforcer.AddPolicy("groups:admins", "orgs/acme", "ns-admin", testNs, "allow", "{}")
	if err != nil {
		t.Fatalf("failed to add mapping: %v", err)
	}

	t.Run("force remove namespace role and mappings", func(t *testing.T) {
		if err := enforcer.ForceRemoveRole(ctx, &authzcore.RoleRef{Name: "ns-admin", Namespace: testNs}); err != nil {
			t.Fatalf("failed to force remove namespace role: %v", err)
		}

		// Verify role is removed from g policies
		gPolicies, err := enforcer.enforcer.GetNamedGroupingPolicy("g")
		if err != nil {
			t.Fatalf("GetNamedGroupingPolicy() error = %v", err)
		}
		for _, policy := range gPolicies {
			if len(policy) >= 3 && policy[0] == "ns-admin" && policy[2] == testNs {
				t.Error("ForceRemoveRole() namespace role still exists in g policies")
			}
		}

		policies, err := enforcer.enforcer.GetPolicy()
		if err != nil {
			t.Fatalf("GetPolicy() error = %v", err)
		}
		mappingCount := 0
		for _, policy := range policies {
			if len(policy) >= 4 && policy[2] == "ns-admin" && policy[3] == testNs {
				mappingCount++
			}
		}
		if mappingCount != 0 {
			t.Errorf("expected 0 mappings after force delete, got %d", mappingCount)
		}
	})
}

// TestCasbinEnforcer_UpdateRole_ClusterRole tests updating cluster-scoped roles
func TestCasbinEnforcer_UpdateRole_ClusterRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("update role with both added and removed actions", func(t *testing.T) {
		roleRules := [][]string{
			{"mixed-update-role", "component:view", "*"},
			{"mixed-update-role", "component:create", "*"},
			{"mixed-update-role", "project:view", "*"},
		}
		_, err := enforcer.enforcer.AddGroupingPolicies(roleRules)
		if err != nil {
			t.Fatalf("AddGroupingPolicies() error = %v", err)
		}

		updatedRole := &authzcore.Role{
			Name:    "mixed-update-role",
			Actions: []string{"component:view", "component:delete"},
		}
		err = enforcer.UpdateRole(ctx, updatedRole)
		if err != nil {
			t.Fatalf("UpdateRole() error = %v", err)
		}

		gPolicies, err := enforcer.enforcer.GetNamedGroupingPolicy("g")
		if err != nil {
			t.Fatalf("GetNamedGroupingPolicy() error = %v", err)
		}

		foundActions := make(map[string]bool)
		for _, policy := range gPolicies {
			if len(policy) >= 2 && policy[0] == "mixed-update-role" {
				foundActions[policy[1]] = true
			}
		}

		expectedActions := map[string]bool{
			"component:view":   true,
			"component:delete": true,
		}

		if len(foundActions) != 2 {
			t.Errorf("UpdateRole() got %d actions, want 2", len(foundActions))
		}

		for action := range expectedActions {
			if !foundActions[action] {
				t.Errorf("UpdateRole() missing expected action: %s", action)
			}
		}

		if foundActions["component:create"] {
			t.Error("UpdateRole() component:create should have been removed")
		}
		if foundActions["project:view"] {
			t.Error("UpdateRole() project:view should have been removed")
		}
	})

	t.Run("update role with empty actions should fail", func(t *testing.T) {
		roleRules := [][]string{
			{"removable-actions-role", "component:view", "*"},
			{"removable-actions-role", "component:create", "*"},
		}
		_, err := enforcer.enforcer.AddGroupingPolicies(roleRules)
		if err != nil {
			t.Fatalf("AddGroupingPolicies() error = %v", err)
		}

		updatedRole := &authzcore.Role{
			Name:    "removable-actions-role",
			Actions: []string{},
		}
		err = enforcer.UpdateRole(ctx, updatedRole)
		if err == nil {
			t.Error("UpdateRole() with empty actions should return error")
		}

		retrieved, err := enforcer.GetRole(ctx, &authzcore.RoleRef{Name: "removable-actions-role"})
		if err != nil {
			t.Fatalf("GetRole() error = %v", err)
		}

		if len(retrieved.Actions) != 2 {
			t.Errorf("UpdateRole() failed but role actions changed, got %d actions, want 2", len(retrieved.Actions))
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
func TestCasbinEnforcer_UpdateRole_NamespacedRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	roleRules := [][]string{
		{"ns-engineer", "component:create", "acme"},
		{"ns-engineer", "component:view", "acme"},
	}
	_, err := enforcer.enforcer.AddGroupingPolicies(roleRules)
	if err != nil {
		t.Fatalf("failed to add namespace role via grouping policies: %v", err)
	}

	t.Run("update namespace role actions", func(t *testing.T) {
		updatedRole := &authzcore.Role{
			Name:      "ns-engineer",
			Namespace: "acme",
			Actions:   []string{"component:create", "component:view", "component:update"},
		}
		if err := enforcer.UpdateRole(ctx, updatedRole); err != nil {
			t.Fatalf("failed to update namespace role: %v", err)
		}

		// Verify update by checking g policies
		// Format: g, <role>, <action>, <namespace>
		gPolicies, err := enforcer.enforcer.GetNamedGroupingPolicy("g")
		if err != nil {
			t.Fatalf("GetNamedGroupingPolicy() error = %v", err)
		}

		foundActions := make(map[string]bool)
		for _, policy := range gPolicies {
			if len(policy) >= 3 && policy[0] == "ns-engineer" && policy[2] == "acme" {
				foundActions[policy[1]] = true
			}
		}

		if len(foundActions) != 3 {
			t.Errorf("UpdateRole() got %d actions, want 3", len(foundActions))
		}

		expectedActions := []string{"component:create", "component:view", "component:update"}
		for _, action := range expectedActions {
			if !foundActions[action] {
				t.Errorf("UpdateRole() missing expected action: %s", action)
			}
		}
	})

	t.Run("update non-existent namespace role", func(t *testing.T) {
		nonExistent := &authzcore.Role{
			Name:      "non-existent",
			Namespace: "acme",
			Actions:   []string{"component:view"},
		}
		err := enforcer.UpdateRole(ctx, nonExistent)
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("expected ErrRoleNotFound, got %v", err)
		}
	})
}

// TestCasbinEnforcer_AddRoleEntitlementMapping tests adding role-entitlement mappings
func TestCasbinEnforcer_AddRoleEntitlementMapping(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	_, err := enforcer.enforcer.AddGroupingPolicy(testRoleName, "component:view", "*")
	if err != nil {
		t.Fatalf("AddGroupingPolicy() error = %v", err)
	}

	mapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "test-group",
		},
		RoleRef: authzcore.RoleRef{Name: testRoleName},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}

	err = enforcer.AddRoleEntitlementMapping(ctx, mapping)
	if err != nil {
		t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
	}

	mappings, err := enforcer.ListRoleEntitlementMappings(ctx, nil)
	if err != nil {
		t.Fatalf("ListRoleEntitlementMappings() error = %v", err)
	}

	found := false
	for _, m := range mappings {
		if m.Entitlement.Claim == "groups" && m.Entitlement.Value == "test-group" && m.RoleRef.Name == "test-role" {
			found = true
			break
		}
	}
	if !found {
		t.Error("AddRoleEntitlementMapping() mapping not found in list")
	}
}

// TestCasbinEnforcer_RemoveRoleEntitlementMapping tests removing role-entitlement mappings
func TestCasbinEnforcer_RemoveRoleEntitlementMapping(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	_, err := enforcer.enforcer.AddGroupingPolicy(testRoleName, "component:view", "*")
	if err != nil {
		t.Fatalf("AddGroupingPolicy() error = %v", err)
	}

	mapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: testEntitlementType,
			Value: testEntitlementValue,
		},
		RoleRef: authzcore.RoleRef{Name: testRoleName},
		Hierarchy: authzcore.ResourceHierarchy{
			Namespace: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}

	if err := enforcer.AddRoleEntitlementMapping(ctx, mapping); err != nil {
		t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
	}

	// Get the mapping ID by listing mappings
	mappings, err := enforcer.ListRoleEntitlementMappings(ctx, nil)
	if err != nil {
		t.Fatalf("ListRoleEntitlementMappings() error = %v", err)
	}

	var mappingID uint
	found := false
	for _, m := range mappings {
		if m.Entitlement.Claim == testEntitlementType && m.Entitlement.Value == testEntitlementValue && m.RoleRef.Name == testRoleName {
			mappingID = m.ID
			found = true
			break
		}
	}
	if !found {
		t.Fatal("AddRoleEntitlementMapping() mapping not found after creation")
	}

	// Remove mapping by ID
	err = enforcer.RemoveRoleEntitlementMapping(ctx, mappingID)
	if err != nil {
		t.Fatalf("RemoveRoleEntitlementMapping() error = %v", err)
	}

	// Verify mapping was removed
	mappings, err = enforcer.ListRoleEntitlementMappings(ctx, nil)
	if err != nil {
		t.Fatalf("ListRoleEntitlementMappings() error = %v", err)
	}

	for _, m := range mappings {
		if m.Entitlement.Claim == testEntitlementType && m.Entitlement.Value == testEntitlementValue && m.RoleRef.Name == testRoleName {
			t.Error("RemoveRoleEntitlementMapping() mapping still exists after removal")
		}
	}
}

// TestCasbinEnforcer_UpdateRoleEntitlementMapping tests updating role-entitlement mappings
func TestCasbinEnforcer_UpdateRoleEntitlementMapping(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	_, err := enforcer.enforcer.AddGroupingPolicy("update-test-role", "component:view", "*")
	if err != nil {
		t.Fatalf("AddGroupingPolicy() error = %v", err)
	}

	t.Run("update existing mapping", func(t *testing.T) {
		mapping := &authzcore.RoleEntitlementMapping{
			Entitlement: authzcore.Entitlement{
				Claim: "group",
				Value: "dev-group",
			},
			RoleRef: authzcore.RoleRef{Name: "update-test-role"},
			Hierarchy: authzcore.ResourceHierarchy{
				Namespace: "acme",
			},
			Effect: authzcore.PolicyEffectAllow,
		}
		if err := enforcer.AddRoleEntitlementMapping(ctx, mapping); err != nil {
			t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
		}

		// Get mapping ID
		mappings, err := enforcer.ListRoleEntitlementMappings(ctx, nil)
		if err != nil {
			t.Fatalf("ListRoleEntitlementMappings() error = %v", err)
		}

		var mappingID uint
		for _, m := range mappings {
			if m.RoleRef.Name == "update-test-role" && m.Entitlement.Value == "dev-group" {
				mappingID = m.ID
				break
			}
		}

		if mappingID == 0 {
			t.Fatal("Could not find created mapping")
		}

		// Update the mapping
		updatedMapping := &authzcore.RoleEntitlementMapping{
			ID: mappingID,
			Entitlement: authzcore.Entitlement{
				Claim: "group",
				Value: "prod-group",
			},
			RoleRef: authzcore.RoleRef{Name: "update-test-role"},
			Hierarchy: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
			},
			Effect: authzcore.PolicyEffectDeny,
		}

		err = enforcer.UpdateRoleEntitlementMapping(ctx, updatedMapping)
		if err != nil {
			t.Fatalf("UpdateRoleEntitlementMapping() error = %v", err)
		}

		// Verify update
		mappings, err = enforcer.ListRoleEntitlementMappings(ctx, nil)
		if err != nil {
			t.Fatalf("ListRoleEntitlementMappings() error = %v", err)
		}

		var found bool
		for _, m := range mappings {
			if m.ID == mappingID {
				found = true
				if m.Entitlement.Value != "prod-group" {
					t.Errorf("UpdateRoleEntitlementMapping() entitlement value = %s, want prod-group", m.Entitlement.Value)
				}
				if m.Hierarchy.Project != "p1" {
					t.Errorf("UpdateRoleEntitlementMapping() project = %s, want p1", m.Hierarchy.Project)
				}
				if m.Effect != authzcore.PolicyEffectDeny {
					t.Errorf("UpdateRoleEntitlementMapping() effect = %s, want deny", m.Effect)
				}
				break
			}
		}

		if !found {
			t.Error("UpdateRoleEntitlementMapping() updated mapping not found")
		}
	})

	t.Run("update non-existent mapping", func(t *testing.T) {
		mapping := &authzcore.RoleEntitlementMapping{
			ID: 999999, // Non-existent ID
			Entitlement: authzcore.Entitlement{
				Claim: "group",
				Value: "test",
			},
			RoleRef: authzcore.RoleRef{Name: "update-test-role"},
			Hierarchy: authzcore.ResourceHierarchy{
				Namespace: "acme",
			},
			Effect: authzcore.PolicyEffectAllow,
		}

		err := enforcer.UpdateRoleEntitlementMapping(ctx, mapping)
		if !errors.Is(err, authzcore.ErrRolePolicyMappingNotFound) {
			t.Errorf("UpdateRoleEntitlementMapping() error = %v, want ErrRolePolicyMappingNotFound", err)
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
