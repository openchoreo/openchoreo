// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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
		SubjectContext: &authzcore.SubjectContext{
			Type:              "user",
			EntitlementClaim:  "groups",
			EntitlementValues: []string{"test"},
		},
		Resource: authzcore.Resource{
			Hierarchy: authzcore.ResourceHierarchy{
				Namespace: "org1",
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
		SubjectContext: &authzcore.SubjectContext{
			Type:              "user",
			EntitlementClaim:  "groups",
			EntitlementValues: []string{"test"},
		},
		Resource: authzcore.Resource{
			Hierarchy: authzcore.ResourceHierarchy{Namespace: "org1"},
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
		SubjectContext: &authzcore.SubjectContext{
			Type:              "user",
			EntitlementClaim:  "groups",
			EntitlementValues: []string{"test"},
		},
		Scope: authzcore.ResourceHierarchy{
			Namespace: "org1",
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

func TestDisabledAuthorizer_DeleteClusterRole(t *testing.T) {
	ctx := context.Background()

	err := disabledAuthorizer.DeleteClusterRole(ctx, "test-role")
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
}

func TestDisabledAuthorizer_DeleteNamespacedRole(t *testing.T) {
	ctx := context.Background()

	err := disabledAuthorizer.DeleteNamespacedRole(ctx, "test-role", "test-ns")
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
}

func TestDisabledAuthorizer_DeleteClusterRoleBinding(t *testing.T) {
	ctx := context.Background()

	err := disabledAuthorizer.DeleteClusterRoleBinding(ctx, "test-binding")
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
}

func TestDisabledAuthorizer_DeleteNamespacedRoleBinding(t *testing.T) {
	ctx := context.Background()

	err := disabledAuthorizer.DeleteNamespacedRoleBinding(ctx, "test-binding", "test-ns")
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
}

func TestDisabledAuthorizer_CreateClusterRole(t *testing.T) {
	ctx := context.Background()

	role, err := disabledAuthorizer.CreateClusterRole(ctx, &openchoreov1alpha1.ClusterAuthzRole{})
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if role != nil {
		t.Errorf("expected nil role, got %v", role)
	}
}

func TestDisabledAuthorizer_GetClusterRole(t *testing.T) {
	ctx := context.Background()

	role, err := disabledAuthorizer.GetClusterRole(ctx, "test-role")
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if role != nil {
		t.Errorf("expected nil role, got %v", role)
	}
}

func TestDisabledAuthorizer_ListClusterRoles(t *testing.T) {
	ctx := context.Background()

	list, err := disabledAuthorizer.ListClusterRoles(ctx, 10, "")
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if list != nil {
		t.Errorf("expected nil list, got %v", list)
	}
}

func TestDisabledAuthorizer_UpdateClusterRole(t *testing.T) {
	ctx := context.Background()

	role, err := disabledAuthorizer.UpdateClusterRole(ctx, &openchoreov1alpha1.ClusterAuthzRole{})
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if role != nil {
		t.Errorf("expected nil role, got %v", role)
	}
}

func TestDisabledAuthorizer_CreateNamespacedRole(t *testing.T) {
	ctx := context.Background()

	role, err := disabledAuthorizer.CreateNamespacedRole(ctx, &openchoreov1alpha1.AuthzRole{})
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if role != nil {
		t.Errorf("expected nil role, got %v", role)
	}
}

func TestDisabledAuthorizer_GetNamespacedRole(t *testing.T) {
	ctx := context.Background()

	role, err := disabledAuthorizer.GetNamespacedRole(ctx, "test-role", "test-ns")
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if role != nil {
		t.Errorf("expected nil role, got %v", role)
	}
}

func TestDisabledAuthorizer_ListNamespacedRoles(t *testing.T) {
	ctx := context.Background()

	list, err := disabledAuthorizer.ListNamespacedRoles(ctx, "test-ns", 10, "")
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if list != nil {
		t.Errorf("expected nil list, got %v", list)
	}
}

func TestDisabledAuthorizer_UpdateNamespacedRole(t *testing.T) {
	ctx := context.Background()

	role, err := disabledAuthorizer.UpdateNamespacedRole(ctx, &openchoreov1alpha1.AuthzRole{})
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if role != nil {
		t.Errorf("expected nil role, got %v", role)
	}
}

func TestDisabledAuthorizer_CreateClusterRoleBinding(t *testing.T) {
	ctx := context.Background()

	binding, err := disabledAuthorizer.CreateClusterRoleBinding(ctx, &openchoreov1alpha1.ClusterAuthzRoleBinding{})
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if binding != nil {
		t.Errorf("expected nil binding, got %v", binding)
	}
}

func TestDisabledAuthorizer_GetClusterRoleBinding(t *testing.T) {
	ctx := context.Background()

	binding, err := disabledAuthorizer.GetClusterRoleBinding(ctx, "test-binding")
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if binding != nil {
		t.Errorf("expected nil binding, got %v", binding)
	}
}

func TestDisabledAuthorizer_ListClusterRoleBindings(t *testing.T) {
	ctx := context.Background()

	list, err := disabledAuthorizer.ListClusterRoleBindings(ctx, 10, "")
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if list != nil {
		t.Errorf("expected nil list, got %v", list)
	}
}

func TestDisabledAuthorizer_UpdateClusterRoleBinding(t *testing.T) {
	ctx := context.Background()

	binding, err := disabledAuthorizer.UpdateClusterRoleBinding(ctx, &openchoreov1alpha1.ClusterAuthzRoleBinding{})
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if binding != nil {
		t.Errorf("expected nil binding, got %v", binding)
	}
}

func TestDisabledAuthorizer_CreateNamespacedRoleBinding(t *testing.T) {
	ctx := context.Background()

	binding, err := disabledAuthorizer.CreateNamespacedRoleBinding(ctx, &openchoreov1alpha1.AuthzRoleBinding{})
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if binding != nil {
		t.Errorf("expected nil binding, got %v", binding)
	}
}

func TestDisabledAuthorizer_GetNamespacedRoleBinding(t *testing.T) {
	ctx := context.Background()

	binding, err := disabledAuthorizer.GetNamespacedRoleBinding(ctx, "test-binding", "test-ns")
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if binding != nil {
		t.Errorf("expected nil binding, got %v", binding)
	}
}

func TestDisabledAuthorizer_ListNamespacedRoleBindings(t *testing.T) {
	ctx := context.Background()

	list, err := disabledAuthorizer.ListNamespacedRoleBindings(ctx, "test-ns", 10, "")
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if list != nil {
		t.Errorf("expected nil list, got %v", list)
	}
}

func TestDisabledAuthorizer_UpdateNamespacedRoleBinding(t *testing.T) {
	ctx := context.Background()

	binding, err := disabledAuthorizer.UpdateNamespacedRoleBinding(ctx, &openchoreov1alpha1.AuthzRoleBinding{})
	if !errors.Is(err, authzcore.ErrAuthzDisabled) {
		t.Errorf("expected ErrAuthzDisabled, got %v", err)
	}
	if binding != nil {
		t.Errorf("expected nil binding, got %v", binding)
	}
}
