// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

const (
	testRoleName         = "test-role"
	testEntitlementType  = "group"
	testEntitlementValue = "test-group"
	user                 = "user"
	serviceAccount       = "service_account"
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

func TestCasbinEnforcer_Evaluate(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	multiRole := &authzcore.Role{
		Name:    "multi-role",
		Actions: []string{"organization:*", "component:*", "project:view", "environment:view"},
	}
	if err := enforcer.AddRole(ctx, multiRole); err != nil {
		t.Fatalf("failed to add multi-role: %v", err)
	}

	multiRoleMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "test-group",
		},
		RoleName: "multi-role",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, multiRoleMapping); err != nil {
		t.Fatalf("failed to add org mapping: %v", err)
	}

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
		RoleName:  "global-admin",
		Hierarchy: authzcore.ResourceHierarchy{
			// Empty hierarchy = global wildcard "*"
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, globalMapping); err != nil {
		t.Fatalf("failed to add global mapping: %v", err)
	}

	componentLevelRole := &authzcore.Role{
		Name:    "component-specific",
		Actions: []string{"component:deploy"},
	}
	if err := enforcer.AddRole(ctx, componentLevelRole); err != nil {
		t.Fatalf("failed to add component-specific role: %v", err)
	}
	componentMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "component-group",
		},
		RoleName: "component-specific",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
			Project:      "p1",
			Component:    "c1",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, componentMapping); err != nil {
		t.Fatalf("failed to add component mapping: %v", err)
	}

	projectLevelRole := &authzcore.Role{
		Name:    "project-specific",
		Actions: []string{"project:create", "component:create"},
	}
	if err := enforcer.AddRole(ctx, projectLevelRole); err != nil {
		t.Fatalf("failed to add project-specific role: %v", err)
	}
	projectMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "project-group",
		},
		RoleName: "project-specific",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
			Project:      "p2",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, projectMapping); err != nil {
		t.Fatalf("failed to add project mapping: %v", err)
	}

	readerRole := &authzcore.Role{
		Name:    "reader",
		Actions: []string{"component:view", "project:view"},
	}
	if err := enforcer.AddRole(ctx, readerRole); err != nil {
		t.Fatalf("failed to add reader role: %v", err)
	}
	writerRole := &authzcore.Role{
		Name:    "writer",
		Actions: []string{"component:create", "project:create"},
	}
	if err := enforcer.AddRole(ctx, writerRole); err != nil {
		t.Fatalf("failed to add writer role: %v", err)
	}
	readerMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "multi-role-group",
		},
		RoleName: "reader",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, readerMapping); err != nil {
		t.Fatalf("failed to add reader mapping: %v", err)
	}
	writerMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "multi-role-group",
		},
		RoleName: "writer",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, writerMapping); err != nil {
		t.Fatalf("failed to add writer mapping: %v", err)
	}

	roleForSubClaim := &authzcore.Role{
		Name:    "sub-claim-role",
		Actions: []string{"component:view"},
	}
	if err := enforcer.AddRole(ctx, roleForSubClaim); err != nil {
		t.Fatalf("failed to add sub-claim-role: %v", err)
	}
	subClaimMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "sub",
			Value: "user-123",
		},
		RoleName: "sub-claim-role",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, subClaimMapping); err != nil {
		t.Fatalf("failed to add sub claim mapping: %v", err)
	}

	serviceAccountRole := &authzcore.Role{
		Name:    "service-role",
		Actions: []string{"component:deploy", "component:view"},
	}
	if err := enforcer.AddRole(ctx, serviceAccountRole); err != nil {
		t.Fatalf("failed to add service-role: %v", err)
	}
	serviceAccountMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "service-account-group",
		},
		RoleName: "service-role",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, serviceAccountMapping); err != nil {
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
			name:              "basic evaluate check",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"test-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
			},
			action: "organization:view",
			want:   true,
			reason: "organization:* at org level should match organization:view",
		},
		{
			name:              "evaluate with hierarchical resource matching",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"test-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "p1",
				Component:    "c1",
			},
			action: "component:view",
			want:   true,
			reason: "policy at org level should apply to components within org",
		},
		{
			name:              "wildcard action match",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"test-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "p1",
				Component:    "c1",
			},
			action: "component:view",
			want:   true,
			reason: "component:* should match component:view",
		},
		{
			name:              "multiple claims - access via at least one group",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"other-group", "test-group", "another-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Component:    "c1",
			},
			action: "component:view",
			want:   true,
			reason: "should grant access if ANY group in array has permission (test-group does)",
		},
		{
			name:              "access denied - action not permitted",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"test-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "p1",
			},
			action: "project:delete",
			want:   false,
			reason: "project:delete not allowed by multi-role actions",
		},
		{
			name:              "access denied - no matching group",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"group1", "group2", "group3"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Component:    "c1",
			},
			action: "component:view",
			want:   false,
			reason: "should deny if NO group in array has permission",
		},
		{
			name:              "access denied - hierarchy out of scope",
			entitlementValues: []string{"test-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme-v2",
				Project:      "p2",
				Component:    "c1",
			},
			action: "component:view",
			want:   false,
			reason: "project-writer role only applies to p1, NOT p2",
		},
		{
			name:              "service account authorization",
			subjectType:       serviceAccount,
			entitlementClaim:  "groups",
			entitlementValues: []string{"service-account-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "p1",
				Component:    "c1",
			},
			action: "component:deploy",
			want:   true,
			reason: "service account should be able to deploy components",
		},
		{
			name:              "authorization with 'sub' claim",
			subjectType:       user,
			entitlementClaim:  "sub",
			entitlementValues: []string{"user-123"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Component:    "c1",
			},
			action: "component:view",
			want:   true,
			reason: "sub claim should work for authorization",
		},
		{
			name:              "authorization with 'sub' claim - denied",
			subjectType:       user,
			entitlementClaim:  "sub",
			entitlementValues: []string{"user-456"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Component:    "c1",
			},
			action: "component:view",
			want:   false,
			reason: "different sub value should be denied",
		},
		{
			name:              "global wildcard - access any organization",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"global-admin-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "any-org",
				Project:      "any-project",
			},
			action: "project:delete",
			want:   true,
			reason: "global wildcard policy should grant access to any resource",
		},
		{
			name:              "component-level policy - exact match",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"component-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "p1",
				Component:    "c1",
			},
			action: "component:deploy",
			want:   true,
			reason: "component-level policy should grant access to exact component",
		},
		{
			name:              "component-level policy - different component denied",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"component-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "p1",
				Component:    "c2",
			},
			action: "component:deploy",
			want:   false,
			reason: "component-level policy should not apply to different component",
		},
		{
			name:              "project-level policy - applies to child components",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"project-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "p2",
				Component:    "c1",
			},
			action: "component:create",
			want:   true,
			reason: "project-level policy should apply to resources within project",
		},
		{
			name:              "multiple roles - read permission",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"multi-role-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Component:    "c1",
			},
			action: "component:view",
			want:   true,
			reason: "user should have read permission from reader role",
		},
		{
			name:              "multiple roles - write permission",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"multi-role-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Component:    "c1",
			},
			action: "component:create",
			want:   true,
			reason: "user should have write permission from writer role",
		},
		{
			name:              "multiple roles - combined permissions for project",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"multi-role-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "p1",
			},
			action: "project:create",
			want:   true,
			reason: "user should have combined permissions from both roles",
		},
		{
			name:              "path matching - no false positive for similar org names",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"test-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme2",
			},
			action: "organization:view",
			want:   false,
			reason: "policy for 'acme' should not match 'acme2'",
		},
		{
			name:              "path matching - no false positive for similar project names",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"project-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "p22",
			},
			action: "project:view",
			want:   false,
			reason: "policy for project 'p2' should not match 'p22'",
		},
		{
			name:              "path matching - exact org match works",
			subjectType:       user,
			entitlementClaim:  "groups",
			entitlementValues: []string{"test-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
			},
			action: "component:view",
			want:   true,
			reason: "exact org match should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use default values if not specified
			subjectType := tt.subjectType
			if subjectType == "" {
				subjectType = user
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

// TestCasbinEnforcer_Evaluate_DenyOverridesAllow tests deny policy logic
func TestCasbinEnforcer_Evaluate_DenyOverridesAllow(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// Setup: Create role
	role := &authzcore.Role{
		Name:    "developer",
		Actions: []string{"component:*", "project:view"},
	}
	if err := enforcer.AddRole(ctx, role); err != nil {
		t.Fatalf("failed to add role: %v", err)
	}

	// Setup: Add allow policy at org level
	allowMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "user-group",
		},
		RoleName: "developer",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, allowMapping); err != nil {
		t.Fatalf("failed to add allow mapping: %v", err)
	}

	// Setup: Add deny policy at project level
	denyMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "user-group",
		},
		RoleName: "developer",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
			Project:      "secret",
		},
		Effect: authzcore.PolicyEffectDeny,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, denyMapping); err != nil {
		t.Fatalf("failed to add deny mapping: %v", err)
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
			name:              "allow in public project",
			entitlementValues: []string{"user-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "public",
				Component:    "c1",
			},
			action: "component:view",
			want:   true,
			reason: "allow policy at org level permits access to public project",
		},
		{
			name:              "deny in secret project (deny overrides allow)",
			entitlementValues: []string{"user-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "secret",
				Component:    "c1",
			},
			action: "component:view",
			want:   false,
			reason: "deny policy at project level overrides allow policy at org level",
		},
		{
			name:              "deny in secret project - component:deploy also denied",
			entitlementValues: []string{"user-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "secret",
				Component:    "c1",
			},
			action: "component:deploy",
			want:   false,
			reason: "deny policy should apply to all component:* actions including deploy",
		},
		{
			name:              "allow in public project - component:create allowed",
			entitlementValues: []string{"user-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "public",
				Component:    "c1",
			},
			action: "component:create",
			want:   true,
			reason: "allow policy at org level permits component:create in non-denied projects",
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
		RoleName: "reader",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	mapping2 := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "dev-group",
		},
		RoleName: "writer",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
			Project:      "p1",
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
						Organization: "acme",
						Project:      "p1",
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
						Organization: "acme",
						Project:      "p1",
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
						Organization: "acme",
						Project:      "p2",
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

func TestCasbinEnforcer_AddRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	role := &authzcore.Role{
		Name:    testRoleName,
		Actions: []string{"component:view", "component:create"},
	}
	err := enforcer.AddRole(ctx, role)
	if err != nil {
		t.Fatalf("AddRole() error = %v", err)
	}

	retrievedRole, err := enforcer.GetRole(ctx, testRoleName)
	if err != nil {
		t.Fatalf("GetRole() error = %v", err)
	}

	if retrievedRole.Name != role.Name {
		t.Errorf("GetRole() name = %s, want %s", retrievedRole.Name, role.Name)
	}

	if len(retrievedRole.Actions) != len(role.Actions) {
		t.Errorf("GetRole() actions count = %d, want %d", len(retrievedRole.Actions), len(role.Actions))
	}
}

func TestCasbinEnforcer_AddRole_Duplicate(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

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
}

func TestCasbinEnforcer_RemoveRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("success - remove role with no mappings", func(t *testing.T) {
		role := &authzcore.Role{
			Name:    "removable-role",
			Actions: []string{"component:view"},
		}

		// Add role
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Remove role
		err := enforcer.RemoveRole(ctx, "removable-role")
		if err != nil {
			t.Fatalf("RemoveRole() error = %v", err)
		}

		// Verify role was removed
		_, err = enforcer.GetRole(ctx, "removable-role")
		if err == nil {
			t.Error("GetRole() after remove should return error")
		}
	})

	t.Run("non-existent role", func(t *testing.T) {
		err := enforcer.RemoveRole(ctx, "non-existent-role")
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("RemoveRole() error = %v, want ErrRoleNotFound", err)
		}
	})

	t.Run("role in use", func(t *testing.T) {
		// Create a role
		role := &authzcore.Role{
			Name:    "in-use-role",
			Actions: []string{"component:view", "component:create"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Create a role-entitlement mapping that uses this role
		mapping := &authzcore.RoleEntitlementMapping{
			Entitlement: authzcore.Entitlement{
				Claim: "group",
				Value: "test-group",
			},
			RoleName: "in-use-role",
			Hierarchy: authzcore.ResourceHierarchy{
				Organization: "acme",
			},
			Effect: authzcore.PolicyEffectAllow,
		}
		if err := enforcer.AddRoleEntitlementMapping(ctx, mapping); err != nil {
			t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
		}

		// Attempt to remove the role - should fail because it's in use
		err := enforcer.RemoveRole(ctx, "in-use-role")
		if !errors.Is(err, authzcore.ErrRoleInUse) {
			t.Errorf("RemoveRole() error = %v, want ErrRoleInUse", err)
		}
	})
}

func TestCasbinEnforcer_GetRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// Create a test role
	testRole := &authzcore.Role{
		Name:    "test-admin",
		Actions: []string{"*", "organization:view", "component:view"},
	}
	if err := enforcer.AddRole(ctx, testRole); err != nil {
		t.Fatalf("AddRole() error = %v", err)
	}

	// Test getting the created role
	role, err := enforcer.GetRole(ctx, "test-admin")
	if err != nil {
		t.Fatalf("GetRole() error = %v", err)
	}

	if role.Name != "test-admin" {
		t.Errorf("GetRole() name = %s, want test-admin", role.Name)
	}

	// Verify it has expected actions
	if len(role.Actions) != 3 {
		t.Errorf("GetRole() actions count = %d, want 3", len(role.Actions))
	}

	hasWildcard := false
	for _, action := range role.Actions {
		if action == "*" {
			hasWildcard = true
			break
		}
	}
	if !hasWildcard {
		t.Error("GetRole() test-admin should have wildcard action")
	}
}

func TestCasbinEnforcer_ListRoles(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// Create test roles
	role1 := &authzcore.Role{
		Name:    "admin-role",
		Actions: []string{"*"},
	}
	role2 := &authzcore.Role{
		Name:    "viewer-role",
		Actions: []string{"component:view", "project:view"},
	}
	role3 := &authzcore.Role{
		Name:    "editor-role",
		Actions: []string{"component:edit", "project:edit"},
	}

	if err := enforcer.AddRole(ctx, role1); err != nil {
		t.Fatalf("AddRole(role1) error = %v", err)
	}
	if err := enforcer.AddRole(ctx, role2); err != nil {
		t.Fatalf("AddRole(role2) error = %v", err)
	}
	if err := enforcer.AddRole(ctx, role3); err != nil {
		t.Fatalf("AddRole(role3) error = %v", err)
	}

	// List roles
	roles, err := enforcer.ListRoles(ctx)
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}

	if len(roles) < 3 {
		t.Errorf("ListRoles() returned %d roles, want 4", len(roles))
	}
}

// ============================================================================
// PAP Tests - Policy Mapping Management
// ============================================================================

func TestCasbinEnforcer_AddRoleEntitlementMapping(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	role := &authzcore.Role{
		Name:    testRoleName,
		Actions: []string{"component:view"},
	}
	if err := enforcer.AddRole(ctx, role); err != nil {
		t.Fatalf("AddRole() error = %v", err)
	}

	mapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "test-group",
		},
		RoleName: testRoleName,
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}

	err := enforcer.AddRoleEntitlementMapping(ctx, mapping)
	if err != nil {
		t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
	}

	mappings, err := enforcer.ListRoleEntitlementMappings(ctx, nil)
	if err != nil {
		t.Fatalf("ListRoleEntitlementMappings() error = %v", err)
	}

	found := false
	for _, m := range mappings {
		if m.Entitlement.Claim == "groups" && m.Entitlement.Value == "test-group" && m.RoleName == "test-role" {
			found = true
			break
		}
	}
	if !found {
		t.Error("AddRoleEntitlementMapping() mapping not found in list")
	}
}

func TestCasbinEnforcer_RemoveRoleEntitlementMapping(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// Add role and mapping
	role := &authzcore.Role{
		Name:    testRoleName,
		Actions: []string{"component:view"},
	}
	if err := enforcer.AddRole(ctx, role); err != nil {
		t.Fatalf("AddRole() error = %v", err)
	}

	mapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: testEntitlementType,
			Value: testEntitlementValue,
		},
		RoleName: testRoleName,
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
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
		if m.Entitlement.Claim == testEntitlementType && m.Entitlement.Value == testEntitlementValue && m.RoleName == testRoleName {
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
		if m.Entitlement.Claim == testEntitlementType && m.Entitlement.Value == testEntitlementValue && m.RoleName == testRoleName {
			t.Error("RemoveRoleEntitlementMapping() mapping still exists after removal")
		}
	}
}

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

func TestCasbinEnforcer_filterPoliciesBySubjectAndScope(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// Setup: Create role and mappings
	role := &authzcore.Role{
		Name:    "viewer",
		Actions: []string{"component:view"},
	}
	if err := enforcer.AddRole(ctx, role); err != nil {
		t.Fatalf("AddRole() error = %v", err)
	}

	mapping1 := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "group",
			Value: "group1",
		},
		RoleName: "viewer",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	mapping2 := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "group",
			Value: "group1",
		},
		RoleName: "viewer",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
			Project:      "p1",
		},
		Effect: authzcore.PolicyEffectDeny,
	}
	mapping3 := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "group",
			Value: "group1",
		},
		RoleName: "viewer",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "other-org",
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
			scopePath:       "org/acme",
			wantPolicyCount: 2,
		},
		{
			name: "filter policies within project scope",
			subjectCtx: &authzcore.SubjectContext{
				Type:              user,
				EntitlementClaim:  "group",
				EntitlementValues: []string{"group1"},
			},
			scopePath:       "org/acme/project/p1",
			wantPolicyCount: 2,
		},
		{
			name: "no matching entitlements",
			subjectCtx: &authzcore.SubjectContext{
				Type:              user,
				EntitlementClaim:  "group",
				EntitlementValues: []string{"nonexistent-group"},
			},
			scopePath:       "org/acme",
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

func TestCasbinEnforcer_GetSubjectProfile(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	viewerRole := &authzcore.Role{
		Name:    "viewer",
		Actions: []string{"component:view", "project:view"},
	}
	editorRole := &authzcore.Role{
		Name:    "editor",
		Actions: []string{"component:*", "project:create"},
	}
	if err := enforcer.AddRole(ctx, viewerRole); err != nil {
		t.Fatalf("AddRole(viewer) error = %v", err)
	}
	if err := enforcer.AddRole(ctx, editorRole); err != nil {
		t.Fatalf("AddRole(editor) error = %v", err)
	}

	// Setup: Add role entitlement mappings
	viewerMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "dev-group",
		},
		RoleName: "editor",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	editorMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "dev-group",
		},
		RoleName: "viewer",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
			Project:      "p1",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	denyMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "groups",
			Value: "dev-group",
		},
		RoleName: "editor",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
			Project:      "secret",
		},
		Effect: authzcore.PolicyEffectDeny,
	}

	if err := enforcer.AddRoleEntitlementMapping(ctx, viewerMapping); err != nil {
		t.Fatalf("AddRoleEntitlementMapping(viewer) error = %v", err)
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, editorMapping); err != nil {
		t.Fatalf("AddRoleEntitlementMapping(editor) error = %v", err)
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, denyMapping); err != nil {
		t.Fatalf("AddRoleEntitlementMapping(deny) error = %v", err)
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
			name: "get profile with org scope",
			request: &authzcore.ProfileRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              user,
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"dev-group"},
				},
				Scope: authzcore.ResourceHierarchy{
					Organization: "acme",
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
				{action: "component:delete", allowedCount: 1, deniedCount: 1},
				{action: "component:update", allowedCount: 1, deniedCount: 1},
				{action: "component:deploy", allowedCount: 1, deniedCount: 1},
				{action: "project:create", allowedCount: 1, deniedCount: 1},
			},
		},
		{
			name: "get profile for scope within an organization",
			request: &authzcore.ProfileRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              user,
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"dev-group"},
				},
				Scope: authzcore.ResourceHierarchy{
					Organization: "acme",
					Project:      "p1",
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
				{action: "component:deploy", allowedCount: 1, deniedCount: 0},
				{action: "component:delete", allowedCount: 1, deniedCount: 0},
				{action: "project:create", allowedCount: 1, deniedCount: 0},
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
					Organization: "acme",
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

	if err := enforcer.AddRole(ctx, viewerRole); err != nil {
		t.Fatalf("AddRole(viewer) error = %v", err)
	}
	if err := enforcer.AddRole(ctx, editorRole); err != nil {
		t.Fatalf("AddRole(editor) error = %v", err)
	}
	if err := enforcer.AddRole(ctx, adminRole); err != nil {
		t.Fatalf("AddRole(admin) error = %v", err)
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
				{resourcePath: "org/acme", roleName: "viewer", effect: "allow"},
				{resourcePath: "org/acme/project/p1", roleName: "editor", effect: "allow"},
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
				{resourcePath: "org/acme", roleName: "editor", effect: "allow"},
				{resourcePath: "org/acme/project/secret", roleName: "editor", effect: "deny"},
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
				{resourcePath: "org/acme", roleName: "admin", effect: "allow"},
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
			name: "multiple policies with same role (duplicates)",
			policies: []policyInfo{
				{resourcePath: "org/acme", roleName: "viewer", effect: "allow"},
				{resourcePath: "org/acme", roleName: "viewer", effect: "allow"},
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

// TestCasbinEnforcer_ForceRemoveRole tests force removal of roles
func TestCasbinEnforcer_ForceRemoveRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	t.Run("force remove role with associated mappings", func(t *testing.T) {
		// Setup: Create role
		role := &authzcore.Role{
			Name:    "force-removable",
			Actions: []string{"component:view"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Add a mapping for this role
		mapping := &authzcore.RoleEntitlementMapping{
			Entitlement: authzcore.Entitlement{
				Claim: "group",
				Value: "test-group",
			},
			RoleName: "force-removable",
			Hierarchy: authzcore.ResourceHierarchy{
				Organization: "acme",
			},
			Effect: authzcore.PolicyEffectAllow,
		}
		if err := enforcer.AddRoleEntitlementMapping(ctx, mapping); err != nil {
			t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
		}

		// Force remove the role
		err := enforcer.ForceRemoveRole(ctx, "force-removable")
		if err != nil {
			t.Fatalf("ForceRemoveRole() error = %v", err)
		}

		// Verify role is gone
		_, err = enforcer.GetRole(ctx, "force-removable")
		if err == nil {
			t.Error("ForceRemoveRole() role still exists after removal")
		}

		// Verify mappings are gone
		mappings, err := enforcer.ListRoleEntitlementMappings(ctx, &authzcore.RoleEntitlementMappingFilter{
			RoleName: strPtr("force-removable"),
		})
		if err != nil {
			t.Fatalf("ListRoleEntitlementMappings() error = %v", err)
		}
		if len(mappings) != 0 {
			t.Errorf("ForceRemoveRole() expected 0 mappings, got %d", len(mappings))
		}
	})

	t.Run("force remove non-existent role", func(t *testing.T) {
		err := enforcer.ForceRemoveRole(ctx, "non-existent")
		if !errors.Is(err, authzcore.ErrRoleNotFound) {
			t.Errorf("ForceRemoveRole() error = %v, want ErrRoleNotFound", err)
		}
	})

	t.Run("force remove role without mappings", func(t *testing.T) {
		// Setup: Create role without mappings
		role := &authzcore.Role{
			Name:    "no-mappings-role",
			Actions: []string{"component:view"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Force remove should succeed
		err := enforcer.ForceRemoveRole(ctx, "no-mappings-role")
		if err != nil {
			t.Fatalf("ForceRemoveRole() error = %v", err)
		}

		// Verify role is gone
		_, err = enforcer.GetRole(ctx, "no-mappings-role")
		if err == nil {
			t.Error("ForceRemoveRole() role still exists after removal")
		}
	})
}

// TestCasbinEnforcer_UpdateRole tests updating existing roles
func TestCasbinEnforcer_UpdateRole(t *testing.T) {
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

		// Update: remove component:create, keep component:view, add component:delete
		updatedRole := &authzcore.Role{
			Name:    "mixed-update-role",
			Actions: []string{"component:view", "component:delete"},
		}
		err := enforcer.UpdateRole(ctx, updatedRole)
		if err != nil {
			t.Fatalf("UpdateRole() error = %v", err)
		}

		// Verify updated actions
		retrieved, err := enforcer.GetRole(ctx, "mixed-update-role")
		if err != nil {
			t.Fatalf("GetRole() error = %v", err)
		}

		if len(retrieved.Actions) != 2 {
			t.Errorf("UpdateRole() got %d actions, want 2", len(retrieved.Actions))
		}

		expectedActions := map[string]bool{
			"component:view":   true,
			"component:delete": true,
		}
		for _, action := range retrieved.Actions {
			if !expectedActions[action] {
				t.Errorf("UpdateRole() unexpected action: %s", action)
			}
		}
	})

	t.Run("update role with empty actions should fail", func(t *testing.T) {
		// Setup: Create role
		role := &authzcore.Role{
			Name:    "removable-actions-role",
			Actions: []string{"component:view", "component:create"},
		}
		if err := enforcer.AddRole(ctx, role); err != nil {
			t.Fatalf("AddRole() error = %v", err)
		}

		// Attempt to update with empty actions - should fail
		updatedRole := &authzcore.Role{
			Name:    "removable-actions-role",
			Actions: []string{},
		}
		err := enforcer.UpdateRole(ctx, updatedRole)
		if err == nil {
			t.Error("UpdateRole() with empty actions should return error")
		}

		// Verify role still has original actions
		retrieved, err := enforcer.GetRole(ctx, "removable-actions-role")
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

// TestCasbinEnforcer_UpdateRoleEntitlementMapping tests updating mappings
func TestCasbinEnforcer_UpdateRoleEntitlementMapping(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// Setup: Create role
	role := &authzcore.Role{
		Name:    "update-test-role",
		Actions: []string{"component:view"},
	}
	if err := enforcer.AddRole(ctx, role); err != nil {
		t.Fatalf("AddRole() error = %v", err)
	}

	t.Run("update existing mapping", func(t *testing.T) {
		mapping := &authzcore.RoleEntitlementMapping{
			Entitlement: authzcore.Entitlement{
				Claim: "group",
				Value: "dev-group",
			},
			RoleName: "update-test-role",
			Hierarchy: authzcore.ResourceHierarchy{
				Organization: "acme",
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
			if m.RoleName == "update-test-role" && m.Entitlement.Value == "dev-group" {
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
			RoleName: "update-test-role",
			Hierarchy: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "p1",
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
			RoleName: "update-test-role",
			Hierarchy: authzcore.ResourceHierarchy{
				Organization: "acme",
			},
			Effect: authzcore.PolicyEffectAllow,
		}

		err := enforcer.UpdateRoleEntitlementMapping(ctx, mapping)
		if !errors.Is(err, authzcore.ErrRolePolicyMappingNotFound) {
			t.Errorf("UpdateRoleEntitlementMapping() error = %v, want ErrRolePolicyMappingNotFound", err)
		}
	})
}

// TestComputeActionsDiff tests the action diff computation
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

// Helper function to create string pointers
func strPtr(s string) *string {
	return &s
}
