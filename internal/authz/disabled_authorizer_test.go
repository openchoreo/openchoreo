// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// sharedDisabledAuthorizer is a single instance used across all disabled authorizer tests
var disabledAuthorizer = NewDisabledAuthorizer(
	slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})),
)

// TestDisabledAuthorizer_Evaluate verifies that authorization checks always return true
func TestDisabledAuthorizer_Evaluate(t *testing.T) {
	ctx := context.Background()

	request := &authzcore.EvaluateRequest{
		Subject: authzcore.Subject{
			JwtToken: "any-jwt",
		},
		Resource: authzcore.Resource{
			Hierarchy: authzcore.ResourceHierarchy{
				Organization: "org1",
			},
		},
		Action: "component:delete",
	}

	decision, err := disabledAuthorizer.Evaluate(ctx, request)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if decision == nil {
		t.Fatal("expected decision to be non-nil")
	}
	if !decision.Decision {
		t.Error("expected decision to be true (access granted)")
	}
}

// TestDisabledAuthorizer_BatchEvaluate verifies that all batch requests return true
func TestDisabledAuthorizer_BatchEvaluate(t *testing.T) {
	ctx := context.Background()
	request := authzcore.EvaluateRequest{
		Subject: authzcore.Subject{JwtToken: "test-jwt"},
		Resource: authzcore.Resource{
			Hierarchy: authzcore.ResourceHierarchy{Organization: "org1"},
		},
		Action: "component:read",
	}
	batchRequest := &authzcore.BatchEvaluateRequest{
		Requests: []authzcore.EvaluateRequest{
			request,
			request,
		},
	}

	response, err := disabledAuthorizer.BatchEvaluate(ctx, batchRequest)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if response == nil {
		t.Fatal("expected response to be non-nil")
	}
	if len(response.Decisions) != 2 {
		t.Errorf("expected 2 decisions, got %d", len(response.Decisions))
	}

	// All decisions should be true
	for i, decision := range response.Decisions {
		if !decision.Decision {
			t.Errorf("decision %d: expected decision to be true, got false", i)
		}
	}
}

// TestDisabledAuthorizer_GetSubjectProfile verifies that wildcard actions are returned
func TestDisabledAuthorizer_GetSubjectProfile(t *testing.T) {
	ctx := context.Background()

	request := &authzcore.ProfileRequest{
		Subject: authzcore.Subject{
			JwtToken: "test-jwt",
		},
		Scope: authzcore.ResourceHierarchy{
			Organization: "org1",
		},
	}

	profile, err := disabledAuthorizer.GetSubjectProfile(ctx, request)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if profile == nil {
		t.Fatal("expected profile to be non-nil")
	}

	wildcardCapability, exists := profile.Capabilities["*"]
	if !exists {
		t.Fatal("expected wildcard (*) action in Capabilities map")
	}
	if len(profile.Capabilities) != 1 {
		t.Errorf("expected 1 capability, got %d", len(profile.Capabilities))
	}
	if len(wildcardCapability.Allowed) != 1 {
		t.Fatalf("expected 1 allowed resource, got %d", len(wildcardCapability.Allowed))
	}
	if wildcardCapability.Allowed[0].Path != "*" {
		t.Errorf("expected wildcard (*) path, got %s", wildcardCapability.Allowed[0].Path)
	}
	if len(wildcardCapability.Denied) != 0 {
		t.Errorf("expected 0 denied resources, got %d", len(wildcardCapability.Denied))
	}
}

// TestDisabledAuthorizer_AddRole verifies PAP methods return ErrAuthzDisabled
func TestDisabledAuthorizer_AddRole(t *testing.T) {
	ctx := context.Background()

	err := disabledAuthorizer.AddRole(ctx, &authzcore.Role{
		Name:    "test-role",
		Actions: []string{"component:read"},
	})
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
}

// TestDisabledAuthorizer_RemoveRole verifies PAP methods return ErrAuthzDisabled
func TestDisabledAuthorizer_RemoveRole(t *testing.T) {
	ctx := context.Background()

	err := disabledAuthorizer.RemoveRole(ctx, "test-role")
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
}

// TestDisabledAuthorizer_GetRole verifies PAP methods return ErrAuthzDisabled
func TestDisabledAuthorizer_GetRole(t *testing.T) {
	ctx := context.Background()

	role, err := disabledAuthorizer.GetRole(ctx, "test-role")
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if role != nil {
		t.Errorf("expected nil role, got %v", role)
	}
}

// TestDisabledAuthorizer_ListRoles verifies PAP methods return ErrAuthzDisabled
func TestDisabledAuthorizer_ListRoles(t *testing.T) {
	ctx := context.Background()

	roles, err := disabledAuthorizer.ListRoles(ctx)
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if roles != nil {
		t.Errorf("expected nil roles, got %v", roles)
	}
}

// TestDisabledAuthorizer_AddRoleEntitlementMapping verifies PAP methods return ErrAuthzDisabled
func TestDisabledAuthorizer_AddRoleEntitlementMapping(t *testing.T) {
	ctx := context.Background()

	err := disabledAuthorizer.AddRoleEntitlementMapping(ctx, &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "group",
			Value: "test-group",
		},
		RoleName:  "test-role",
		Hierarchy: authzcore.ResourceHierarchy{Organization: "org1"},
		Effect:    authzcore.PolicyEffectAllow,
	})
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
}

// TestDisabledAuthorizer_RemoveRoleEntitlementMapping verifies PAP methods return ErrAuthzDisabled
func TestDisabledAuthorizer_RemoveRoleEntitlementMapping(t *testing.T) {
	ctx := context.Background()

	err := disabledAuthorizer.RemoveRoleEntitlementMapping(ctx, &authzcore.RoleEntitlementMapping{
		Entitlement: authzcore.Entitlement{
			Claim: "group",
			Value: "test-group",
		},
		RoleName:  "test-role",
		Hierarchy: authzcore.ResourceHierarchy{Organization: "org1"},
		Effect:    authzcore.PolicyEffectAllow,
	})
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
}

// TestDisabledAuthorizer_ListActions verifies PAP methods return ErrAuthzDisabled
func TestDisabledAuthorizer_ListActions(t *testing.T) {
	ctx := context.Background()

	actions, err := disabledAuthorizer.ListActions(ctx)
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if actions != nil {
		t.Errorf("expected nil actions, got %v", actions)
	}
}
