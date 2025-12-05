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
	"github.com/openchoreo/openchoreo/internal/authz/usertype"
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
		UserTypeConfigs: []usertype.UserTypeConfig{
			{
				Type:        authzcore.SubjectTypeUser,
				DisplayName: "Human User",
				Priority:    1,
				Entitlement: usertype.EntitlementConfig{
					Claim:       "group",
					DisplayName: "User Group",
				},
			},
			{
				Type:        authzcore.SubjectTypeServiceAccount,
				DisplayName: "Service Account",
				Priority:    2,
				Entitlement: usertype.EntitlementConfig{
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
		EntitlementValue: "test-group",
		RoleName:         "multi-role",
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
		EntitlementValue: "user-group",
		RoleName:         "developer",
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
		EntitlementValue: "user-group",
		RoleName:         "developer",
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
		EntitlementValue: "dev-group",
		RoleName:         "reader",
		Hierarchy: authzcore.ResourceHierarchy{
			Organization: "acme",
		},
		Effect: authzcore.PolicyEffectAllow,
	}
	mapping2 := &authzcore.RoleEntitlementMapping{
		EntitlementValue: "dev-group",
		RoleName:         "writer",
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
		EntitlementValue: "test-group",
		RoleName:         "test-role",
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
		if m.EntitlementValue == "test-group" && m.RoleName == "test-role" {
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
		EntitlementValue: "test-group",
		RoleName:         "test-role",
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
		if m.EntitlementValue == "test-group" && m.RoleName == "test-role" {
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
