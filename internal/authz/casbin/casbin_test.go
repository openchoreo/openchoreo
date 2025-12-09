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

	"github.com/golang-jwt/jwt/v5"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
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
		UserTypeConfigs: []authzcore.UserTypeConfig{
			{
				Type:        authzcore.SubjectTypeUser,
				DisplayName: "Human User",
				Priority:    1,
				Entitlement: authzcore.EntitlementConfig{
					Claim:       "group",
					DisplayName: "User Group",
				},
			},
			{
				Type:        authzcore.SubjectTypeServiceAccount,
				DisplayName: "Service Account",
				Priority:    2,
				Entitlement: authzcore.EntitlementConfig{
					Claim:       "service_account",
					DisplayName: "Service Account ID",
				},
			},
		},
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

// createTestJWT creates a JWT token with the specified claims
func createTestJWT(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("failed to create test JWT: %v", err)
	}

	return tokenString
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

	orgMapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "group",
			Value: "test-group",
		},
		RoleName: "multi-role",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	if err := enforcer.AddRoleEntitlementMapping(ctx, orgMapping); err != nil {
		t.Fatalf("failed to add org mapping: %v", err)
	}

	tests := []struct {
		name      string
		jwtClaims jwt.MapClaims
		resource  authzcore.ResourceHierarchy
		action    string
		want      bool
		reason    string
	}{
		{
			name:      "basic evaluate check",
			jwtClaims: jwt.MapClaims{"group": "test-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
			},
			action: "organization:view",
			want:   true,
			reason: "organization:* at org level should match organization:view",
		},
		{
			name:      "evaluate with hierarchical resource matching",
			jwtClaims: jwt.MapClaims{"group": "test-group"},
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
			name:      "wildcard action match",
			jwtClaims: jwt.MapClaims{"group": "test-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Component:    "c1",
			},
			action: "component:read",
			want:   true,
			reason: "component:* should match component:read",
		},
		{
			name:      "multiple claims - access via at least one group",
			jwtClaims: jwt.MapClaims{"group": []interface{}{"other-group", "test-group", "another-group"}},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Component:    "c1",
			},
			action: "component:read",
			want:   true,
			reason: "should grant access if ANY group in array has permission (test-group does)",
		},
		{
			name:      "access denied - action not permitted",
			jwtClaims: jwt.MapClaims{"group": "test-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "p1",
			},
			action: "project:delete",
			want:   false,
			reason: "project:delete not allowed by multi-role actions",
		},
		{
			name:      "access denied - no matching group",
			jwtClaims: jwt.MapClaims{"group": []interface{}{"group1", "group2", "group3"}},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Component:    "c1",
			},
			action: "component:read",
			want:   false,
			reason: "should deny if NO group in array has permission",
		},
		{
			name:      "access denied - hierarchy out of scope",
			jwtClaims: jwt.MapClaims{"group": "project-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "p2",
				Component:    "c1",
			},
			action: "component:write",
			want:   false,
			reason: "project-writer role only applies to p1, NOT p2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testToken := createTestJWT(t, tt.jwtClaims)

			request := &authzcore.EvaluateRequest{
				Subject: authzcore.Subject{
					JwtToken: testToken,
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
			Claim: "group",
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
			Claim: "group",
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
		name      string
		jwtClaims jwt.MapClaims
		resource  authzcore.ResourceHierarchy
		action    string
		want      bool
		reason    string
	}{
		{
			name:      "allow in public project",
			jwtClaims: jwt.MapClaims{"group": "user-group"},
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
			name:      "deny in secret project (deny overrides allow)",
			jwtClaims: jwt.MapClaims{"group": "user-group"},
			resource: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "secret",
				Component:    "c1",
			},
			action: "component:view",
			want:   false,
			reason: "deny policy at project level overrides allow policy at org level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jwtToken := createTestJWT(t, tt.jwtClaims)

			request := &authzcore.EvaluateRequest{
				Subject: authzcore.Subject{
					JwtToken: jwtToken,
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
		Actions: []string{"component:read"},
	}
	role2 := &authzcore.Role{
		Name:    "writer",
		Actions: []string{"component:write"},
	}

	if err := enforcer.AddRole(ctx, role1); err != nil {
		t.Fatalf("failed to add role1: %v", err)
	}
	if err := enforcer.AddRole(ctx, role2); err != nil {
		t.Fatalf("failed to add role2: %v", err)
	}

	mapping1 := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "group",
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
			Claim: "group",
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
				Subject: authzcore.Subject{
					JwtToken: createTestJWT(t, jwt.MapClaims{"group": "dev-group"}),
				},
				Resource: authzcore.Resource{
					Type: "component",
					Hierarchy: authzcore.ResourceHierarchy{
						Organization: "acme",
						Project:      "p1",
					},
				},
				Action: "component:read",
			},
			{
				Subject: authzcore.Subject{
					JwtToken: createTestJWT(t, jwt.MapClaims{"group": "dev-group"}),
				},
				Resource: authzcore.Resource{
					Type: "component",
					Hierarchy: authzcore.ResourceHierarchy{
						Organization: "acme",
						Project:      "p1",
					},
				},
				Action: "component:write",
			},
			{
				Subject: authzcore.Subject{
					JwtToken: createTestJWT(t, jwt.MapClaims{"group": "dev-group"}),
				},
				Resource: authzcore.Resource{
					Type: "component",
					Hierarchy: authzcore.ResourceHierarchy{
						Organization: "acme",
						Project:      "p2",
					},
				},
				Action: "component:write",
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
		Name:    "test-role",
		Actions: []string{"component:read", "component:write"},
	}
	err := enforcer.AddRole(ctx, role)
	if err != nil {
		t.Fatalf("AddRole() error = %v", err)
	}

	retrievedRole, err := enforcer.GetRole(ctx, "test-role")
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
		Actions: []string{"component:read"},
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

	role := &authzcore.Role{
		Name:    "removable-role",
		Actions: []string{"component:read"},
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
}

func TestCasbinEnforcer_RemoveRole_NonExistent(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	err := enforcer.RemoveRole(ctx, "non-existent-role")
	if !errors.Is(err, authzcore.ErrRoleNotFound) {
		t.Errorf("RemoveRole() error = %v, want ErrRoleNotFound", err)
	}
}

func TestCasbinEnforcer_GetRole(t *testing.T) {
	enforcer := setupTestEnforcer(t)
	ctx := context.Background()

	// Create a test role
	testRole := &authzcore.Role{
		Name:    "test-admin",
		Actions: []string{"*", "organization:view", "component:read"},
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

	if len(roles) != 4 {
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
		Name:    "test-role",
		Actions: []string{"component:read"},
	}
	if err := enforcer.AddRole(ctx, role); err != nil {
		t.Fatalf("AddRole() error = %v", err)
	}

	mapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "group",
			Value: "test-group",
		},
		RoleName: "test-role",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}

	err := enforcer.AddRoleEntitlementMapping(ctx, mapping)
	if err != nil {
		t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
	}

	mappings, err := enforcer.ListRoleEntitlementMappings(ctx)
	if err != nil {
		t.Fatalf("ListRoleEntitlementMappings() error = %v", err)
	}

	found := false
	for _, m := range mappings {
		if m.Entitlement.Claim == "group" && m.Entitlement.Value == "test-group" && m.RoleName == "test-role" {
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
		Name:    "test-role",
		Actions: []string{"component:read"},
	}
	if err := enforcer.AddRole(ctx, role); err != nil {
		t.Fatalf("AddRole() error = %v", err)
	}

	mapping := &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "group",
			Value: "test-group",
		},
		RoleName: "test-role",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}

	if err := enforcer.AddRoleEntitlementMapping(ctx, mapping); err != nil {
		t.Fatalf("AddRoleEntitlementMapping() error = %v", err)
	}

	// Remove mapping
	err := enforcer.RemoveRoleEntitlementMapping(ctx, mapping)
	if err != nil {
		t.Fatalf("RemoveRoleEntitlementMapping() error = %v", err)
	}

	// Verify mapping was removed
	mappings, err := enforcer.ListRoleEntitlementMappings(ctx)
	if err != nil {
		t.Fatalf("ListRoleEntitlementMappings() error = %v", err)
	}

	for _, m := range mappings {
		if m.Entitlement.Claim == "group" && m.Entitlement.Value == "test-group" && m.RoleName == "test-role" {
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
		Actions: []string{"component:read"},
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
				Type:              authzcore.SubjectTypeUser,
				EntitlementClaim:  "group",
				EntitlementValues: []string{"group1"},
			},
			scopePath:       "org/acme",
			wantPolicyCount: 2, // Only policies within acme org
		},
		{
			name: "filter policies within project scope",
			subjectCtx: &authzcore.SubjectContext{
				Type:              authzcore.SubjectTypeUser,
				EntitlementClaim:  "group",
				EntitlementValues: []string{"group1"},
			},
			scopePath:       "org/acme/project/p1",
			wantPolicyCount: 2, // Both org-level and project-level policies match
		},
		{
			name: "no matching entitlements",
			subjectCtx: &authzcore.SubjectContext{
				Type:              authzcore.SubjectTypeUser,
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
			Claim: "group",
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
			Claim: "group",
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
			Claim: "group",
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
			name: "get profile for user with viewer role at org level",
			request: &authzcore.ProfileRequest{
				Subject: authzcore.Subject{
					JwtToken: createTestJWT(t, jwt.MapClaims{"group": "dev-group"}),
				},
				Scope: authzcore.ResourceHierarchy{
					Organization: "acme",
				},
			},
			wantErr: false,
			expectedUser: authzcore.SubjectContext{
				Type:              authzcore.SubjectTypeUser,
				EntitlementClaim:  "group",
				EntitlementValues: []string{"dev-group"},
			},
			expectedCapabilities: []expectedCapability{
				{action: "component:view", allowedCount: 2, deniedCount: 1},
				{action: "project:view", allowedCount: 1, deniedCount: 0},
				{action: "component:create", allowedCount: 1, deniedCount: 1},
				{action: "component:update", allowedCount: 1, deniedCount: 1},
				{action: "component:deploy", allowedCount: 1, deniedCount: 1},
				{action: "component:promote", allowedCount: 1, deniedCount: 1},
				{action: "project:create", allowedCount: 1, deniedCount: 1},
			},
		},
		{
			name: "get profile with project scope",
			request: &authzcore.ProfileRequest{
				Subject: authzcore.Subject{
					JwtToken: createTestJWT(t, jwt.MapClaims{"group": "dev-group"}),
				},
				Scope: authzcore.ResourceHierarchy{
					Organization: "acme",
					Project:      "p1",
				},
			},
			wantErr: false,
			expectedUser: authzcore.SubjectContext{
				Type:              authzcore.SubjectTypeUser,
				EntitlementClaim:  "group",
				EntitlementValues: []string{"dev-group"},
			},
			expectedCapabilities: []expectedCapability{
				// Within p1 scope: no denied policies apply (secret is different project)
				{action: "component:view", allowedCount: 2, deniedCount: 0},
				{action: "project:view", allowedCount: 1, deniedCount: 0},
				{action: "component:create", allowedCount: 1, deniedCount: 0},
				{action: "component:update", allowedCount: 1, deniedCount: 0},
				{action: "component:deploy", allowedCount: 1, deniedCount: 0},
				{action: "component:promote", allowedCount: 1, deniedCount: 0},
				{action: "project:create", allowedCount: 1, deniedCount: 0},
			},
		},
		{
			name: "get profile for user with no permissions",
			request: &authzcore.ProfileRequest{
				Subject: authzcore.Subject{
					JwtToken: createTestJWT(t, jwt.MapClaims{"group": "no-permissions-group"}),
				},
				Scope: authzcore.ResourceHierarchy{
					Organization: "acme",
				},
			},
			wantErr:             false,
			wantEmptyCapability: true,
			expectedUser: authzcore.SubjectContext{
				Type:              authzcore.SubjectTypeUser,
				EntitlementClaim:  "group",
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
