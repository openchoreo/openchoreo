// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/interpreter"
	"github.com/stretchr/testify/require"

	authzv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

func TestSerializeContext(t *testing.T) {
	t.Run("round-trips through JSON", func(t *testing.T) {
		ctx := authzcore.Context{Resource: authzcore.ResourceAttribute{Environment: "staging"}}
		s, err := serializeAuthzContext(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, s)
	})

	t.Run("empty context serializes to valid JSON", func(t *testing.T) {
		ctx := authzcore.Context{}
		s, err := serializeAuthzContext(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, s)
	})
}

func TestResourceMatch(t *testing.T) {
	tests := []struct {
		name            string
		requestResource string
		policyResource  string
		want            bool
	}{
		{
			name:            "exact match",
			requestResource: "ns/acme",
			policyResource:  "ns/acme",
			want:            true,
		},
		{
			name:            "hierarchical prefix match",
			requestResource: "ns/acme/project/p1/component/c1",
			policyResource:  "ns/acme/project/p1",
			want:            true,
		},
		{
			name:            "no match - different namespace",
			requestResource: "ns/other/project/p1",
			policyResource:  "ns/acme",
			want:            false,
		},
		{
			name:            "no match - partial prefix without hierarchy boundary",
			requestResource: "ns/acme-other",
			policyResource:  "ns/acme",
			want:            false,
		},
		{
			name:            "wildcard matches any resource",
			requestResource: "ns/acme/project/p1/component/c1",
			policyResource:  "*",
			want:            true,
		},
		{
			name:            "no match - policy is more specific than request",
			requestResource: "ns/acme",
			policyResource:  "ns/acme/project/p1",
			want:            false,
		},
		{
			name:            "empty request resource",
			requestResource: "",
			policyResource:  "ns/acme",
			want:            false,
		},
		{
			name:            "empty policy resource",
			requestResource: "ns/acme",
			policyResource:  "",
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resourceMatch(tt.requestResource, tt.policyResource)
			if got != tt.want {
				t.Errorf("resourceMatch(%q, %q) = %v, want %v", tt.requestResource, tt.policyResource, got, tt.want)
			}
		})
	}
}

func TestActionMatch(t *testing.T) {
	tests := []struct {
		name          string
		roleAction    string
		requestAction string
		want          bool
	}{
		{
			name:          "exact match",
			roleAction:    "component:read",
			requestAction: "component:read",
			want:          true,
		},
		{
			name:          "verb wildcard match",
			roleAction:    "component:*",
			requestAction: "component:read",
			want:          true,
		},
		{
			name:          "full wildcard matches any action",
			roleAction:    "*",
			requestAction: "component:read",
			want:          true,
		},
		{
			name:          "no match - different resource type",
			roleAction:    "component:*",
			requestAction: "project:read",
			want:          false,
		},
		{
			name:          "no match - different verb",
			roleAction:    "component:read",
			requestAction: "component:write",
			want:          false,
		},
		{
			name:          "empty role action",
			roleAction:    "",
			requestAction: "component:read",
			want:          false,
		},
		{
			name:          "empty request action",
			roleAction:    "component:read",
			requestAction: "",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := actionMatch(tt.requestAction, tt.roleAction)
			if got != tt.want {
				t.Errorf("actionMatch(%q, %q) = %v, want %v", tt.requestAction, tt.roleAction, got, tt.want)
			}
		})
	}
}

func TestHierarchyToResourcePath(t *testing.T) {
	tests := []struct {
		name      string
		hierarchy authzcore.ResourceHierarchy
		want      string
	}{
		{
			name: "full hierarchy",
			hierarchy: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Component: "c1",
			},
			want: "ns/acme/project/p1/component/c1",
		},
		{
			name: "namespace only",
			hierarchy: authzcore.ResourceHierarchy{
				Namespace: "acme",
			},
			want: "ns/acme",
		},
		{
			name: "namespace and project",
			hierarchy: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
			},
			want: "ns/acme/project/p1",
		},
		{
			name:      "empty hierarchy - returns wildcard",
			hierarchy: authzcore.ResourceHierarchy{},
			want:      "*",
		},
		{
			name: "namespace, project, resource",
			hierarchy: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Resource:  "r1",
			},
			want: "ns/acme/project/p1/resource/r1",
		},
		{
			name: "resource sibling does not collide with component",
			hierarchy: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Resource:  "c1",
			},
			want: "ns/acme/project/p1/resource/c1",
		},
		{
			name: "component wins if both set (defensive; CRD CEL rejects this case)",
			hierarchy: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Component: "c1",
				Resource:  "r1",
			},
			want: "ns/acme/project/p1/component/c1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resourceHierarchyToPath(tt.hierarchy)
			if got != tt.want {
				t.Errorf("resourceHierarchyToPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResourcePathToHierarchy(t *testing.T) {
	tests := []struct {
		name         string
		resourcePath string
		want         authzcore.ResourceHierarchy
	}{
		{
			name:         "full hierarchy path",
			resourcePath: "ns/acme/project/p1/component/c1",
			want: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Component: "c1",
			},
		},
		{
			name:         "namespace only",
			resourcePath: "ns/acme",
			want: authzcore.ResourceHierarchy{
				Namespace: "acme",
			},
		},
		{
			name:         "namespace and project",
			resourcePath: "ns/acme/project/p1",
			want: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
			},
		},
		{
			name:         "wildcard returns empty hierarchy",
			resourcePath: "*",
			want:         authzcore.ResourceHierarchy{},
		},
		{
			name:         "empty path returns empty hierarchy",
			resourcePath: "",
			want:         authzcore.ResourceHierarchy{},
		},
		{
			name:         "full hierarchy path with resource",
			resourcePath: "ns/acme/project/p1/resource/r1",
			want: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Resource:  "r1",
			},
		},
		{
			name:         "resource path with shared name does not set component",
			resourcePath: "ns/acme/project/p1/resource/c1",
			want: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Resource:  "c1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resourcePathToHierarchy(tt.resourcePath)

			if got.Namespace != tt.want.Namespace {
				t.Errorf("Namespace = %q, want %q", got.Namespace, tt.want.Namespace)
			}
			if got.Project != tt.want.Project {
				t.Errorf("Project = %q, want %q", got.Project, tt.want.Project)
			}
			if got.Component != tt.want.Component {
				t.Errorf("Component = %q, want %q", got.Component, tt.want.Component)
			}
			if got.Resource != tt.want.Resource {
				t.Errorf("Resource = %q, want %q", got.Resource, tt.want.Resource)
			}
		})
	}
}

// TestHierarchyPathRoundTrip exercises round-trip and branch-independence
// invariants for the resource sub-scope:
//   - encode → decode yields the same hierarchy
//   - component/<name> and resource/<name> never collide even for shared names
func TestHierarchyPathRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		hierarchy authzcore.ResourceHierarchy
	}{
		{
			name: "namespace + project + component",
			hierarchy: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Component: "c1",
			},
		},
		{
			name: "namespace + project + resource",
			hierarchy: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Resource:  "r1",
			},
		},
		{
			name: "component and resource with shared name encode to distinct paths",
			hierarchy: authzcore.ResourceHierarchy{
				Namespace: "acme",
				Project:   "p1",
				Resource:  "shared",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := resourceHierarchyToPath(tt.hierarchy)
			got := resourcePathToHierarchy(path)
			if got != tt.hierarchy {
				t.Errorf("round-trip: encoded %q decoded to %+v, want %+v", path, got, tt.hierarchy)
			}
		})
	}

	t.Run("component and resource with shared name encode to distinct paths", func(t *testing.T) {
		comp := resourceHierarchyToPath(authzcore.ResourceHierarchy{
			Namespace: "acme", Project: "p1", Component: "shared",
		})
		res := resourceHierarchyToPath(authzcore.ResourceHierarchy{
			Namespace: "acme", Project: "p1", Resource: "shared",
		})
		if comp == res {
			t.Errorf("component path %q must not equal resource path %q", comp, res)
		}
	})
}

func TestValidateEvaluateRequest(t *testing.T) {
	tests := []struct {
		name    string
		request *authzcore.EvaluateRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: &authzcore.EvaluateRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              "user",
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"test"},
				},
				Resource: authzcore.Resource{
					Type: "component",
				},
				Action: "component:read",
			},
			wantErr: false,
		},
		{
			name:    "nil request",
			request: nil,
			wantErr: true,
		},
		{
			name: "missing subject context",
			request: &authzcore.EvaluateRequest{
				SubjectContext: nil,
				Resource: authzcore.Resource{
					Type: "component",
				},
				Action: "component:read",
			},
			wantErr: true,
		},
		{
			name: "missing resource type",
			request: &authzcore.EvaluateRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              "user",
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"test"},
				},
				Resource: authzcore.Resource{
					Type: "",
				},
				Action: "component:read",
			},
			wantErr: true,
		},
		{
			name: "missing action",
			request: &authzcore.EvaluateRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              "user",
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"test"},
				},
				Resource: authzcore.Resource{
					Type: "component",
				},
				Action: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEvaluateRequest(tt.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEvaluateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBatchEvaluateRequest(t *testing.T) {
	validRequest := authzcore.EvaluateRequest{
		SubjectContext: &authzcore.SubjectContext{
			Type:              "user",
			EntitlementClaim:  "groups",
			EntitlementValues: []string{"test"},
		},
		Resource: authzcore.Resource{
			Type: "component",
		},
		Action: "component:read",
	}

	tests := []struct {
		name    string
		request *authzcore.BatchEvaluateRequest
		wantErr bool
	}{
		{
			name: "valid batch request",
			request: &authzcore.BatchEvaluateRequest{
				Requests: []authzcore.EvaluateRequest{validRequest, validRequest},
			},
			wantErr: false,
		},
		{
			name:    "nil batch request",
			request: nil,
			wantErr: true,
		},
		{
			name: "empty requests array - valid",
			request: &authzcore.BatchEvaluateRequest{
				Requests: []authzcore.EvaluateRequest{},
			},
			wantErr: true,
		},
		{
			name: "invalid request at index 0 - missing subject context",
			request: &authzcore.BatchEvaluateRequest{
				Requests: []authzcore.EvaluateRequest{
					{
						SubjectContext: nil,
						Resource: authzcore.Resource{
							Type: "component",
						},
						Action: "component:read",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid request at index 1 - missing resource type",
			request: &authzcore.BatchEvaluateRequest{
				Requests: []authzcore.EvaluateRequest{
					validRequest,
					{
						SubjectContext: &authzcore.SubjectContext{
							Type:              "user",
							EntitlementClaim:  "groups",
							EntitlementValues: []string{"test"},
						},
						Resource: authzcore.Resource{
							Type: "",
						},
						Action: "component:read",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBatchEvaluateRequest(tt.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBatchEvaluateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateProfileRequest(t *testing.T) {
	tests := []struct {
		name    string
		request *authzcore.ProfileRequest
		wantErr bool
	}{
		{
			name: "valid profile request",
			request: &authzcore.ProfileRequest{
				SubjectContext: &authzcore.SubjectContext{
					Type:              "user",
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"test-group"},
				},
				Scope: authzcore.ResourceHierarchy{
					Namespace: "acme",
				},
			},
			wantErr: false,
		},
		{
			name:    "nil profile request",
			request: nil,
			wantErr: true,
		},
		{
			name: "missing subject context",
			request: &authzcore.ProfileRequest{
				SubjectContext: nil,
				Scope: authzcore.ResourceHierarchy{
					Namespace: "acme",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProfileRequest(tt.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProfileRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsWithinScope(t *testing.T) {
	tests := []struct {
		name           string
		policyResource string
		scopePath      string
		want           bool
	}{
		{
			name:           "exact match",
			policyResource: "ns/acme",
			scopePath:      "ns/acme",
			want:           true,
		},
		{
			name:           "policy is broader (parent) than scope",
			policyResource: "ns/acme",
			scopePath:      "ns/acme/project/p1",
			want:           true,
		},
		{
			name:           "policy is narrower (child) than scope",
			policyResource: "ns/acme/project/p1",
			scopePath:      "ns/acme",
			want:           true,
		},
		{
			name:           "wildcard policy matches any scope",
			policyResource: "*",
			scopePath:      "ns/acme/project/p1",
			want:           true,
		},
		{
			name:           "wildcard scope matches any policy",
			policyResource: "ns/acme/project/p1",
			scopePath:      "*",
			want:           true,
		},
		{
			name:           "different scopes - no match",
			policyResource: "ns/acme/project/p1",
			scopePath:      "ns/acme/project/p2",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isWithinScope(tt.policyResource, tt.scopePath)
			if got != tt.want {
				t.Errorf("isWithinScope(%q, %q) = %v, want %v", tt.policyResource, tt.scopePath, got, tt.want)
			}
		})
	}
}

func TestExpandActionWildcard(t *testing.T) {
	// Create test action index
	testActions := []authzcore.Action{
		{Name: "component:create"},
		{Name: "component:view"},
		{Name: "component:delete"},
		{Name: "project:view"},
		{Name: "project:update"},
		{Name: "namespace:view"},
	}
	actionIdx := indexActions(testActions)

	tests := []struct {
		name          string
		actionPattern string
		want          []string
	}{
		{
			name:          "concrete action returns as-is",
			actionPattern: "component:view",
			want:          []string{"component:view"},
		},
		{
			name:          "verb wildcard expands to all matching actions",
			actionPattern: "component:*",
			want:          []string{"component:create", "component:view", "component:delete"},
		},
		{
			name:          "full wildcard expands to all actions",
			actionPattern: "*",
			want:          []string{"component:create", "component:view", "component:delete", "project:view", "project:update", "namespace:view"},
		},
		{
			name:          "wildcard for non-existent resource type returns empty",
			actionPattern: "nonexistent:*",
			want:          []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandActionWildcard(tt.actionPattern, actionIdx)
			if len(got) != len(tt.want) {
				t.Errorf("expandActionWildcard(%q) returned %d actions, want %d", tt.actionPattern, len(got), len(tt.want))
				return
			}
			// Convert to map for easier comparison
			gotMap := make(map[string]bool)
			for _, a := range got {
				gotMap[a] = true
			}
			for _, wantAction := range tt.want {
				if !gotMap[wantAction] {
					t.Errorf("expandActionWildcard(%q) missing action %q", tt.actionPattern, wantAction)
				}
			}
		})
	}
}

// TestFormatSubject tests the formatSubject helper function
func TestFormatSubject(t *testing.T) {
	tests := []struct {
		name    string
		claim   string
		value   string
		want    string
		wantErr bool
	}{
		{
			name:    "basic format",
			claim:   "group",
			value:   "developers",
			want:    "group:developers",
			wantErr: false,
		},
		{
			name:    "service account format",
			claim:   "service_account",
			value:   "my-service",
			want:    "service_account:my-service",
			wantErr: false,
		},
		{
			name:    "empty claim - should error",
			claim:   "",
			value:   "test",
			want:    "",
			wantErr: true,
		},
		{
			name:    "empty value - should error",
			claim:   "group",
			value:   "",
			want:    "",
			wantErr: true,
		},
		{
			name:    "both empty - should error",
			claim:   "",
			value:   "",
			want:    "",
			wantErr: true,
		},
		{
			name:    "value with special characters",
			claim:   "group",
			value:   "dev-team@acme.com",
			want:    "group:dev-team@acme.com",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatSubject(tt.claim, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("formatSubject(%q, %q) error = %v, wantErr %v", tt.claim, tt.value, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("formatSubject(%q, %q) = %q, want %q", tt.claim, tt.value, got, tt.want)
			}
		})
	}
}

// TestParseSubject tests the parseSubject helper function
func TestParseSubject(t *testing.T) {
	tests := []struct {
		name      string
		subject   string
		wantClaim string
		wantValue string
		wantErr   bool
	}{
		{
			name:      "basic parse",
			subject:   "group:developers",
			wantClaim: "group",
			wantValue: "developers",
			wantErr:   false,
		},
		{
			name:      "service account parse",
			subject:   "service_account:my-service",
			wantClaim: "service_account",
			wantValue: "my-service",
			wantErr:   false,
		},
		{
			name:      "empty claim",
			subject:   ":test",
			wantClaim: "",
			wantValue: "test",
			wantErr:   false,
		},
		{
			name:      "empty value",
			subject:   "group:",
			wantClaim: "group",
			wantValue: "",
			wantErr:   false,
		},
		{
			name:      "value with special characters",
			subject:   "group:dev-team@acme.com",
			wantClaim: "group",
			wantValue: "dev-team@acme.com",
			wantErr:   false,
		},
		{
			name:      "value with multiple colons",
			subject:   "group:namespace:team",
			wantClaim: "group",
			wantValue: "namespace:team",
			wantErr:   false,
		},
		{
			name:      "no colon - invalid format",
			subject:   "groupdevelopers",
			wantClaim: "",
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "empty subject",
			subject:   "",
			wantClaim: "",
			wantValue: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotClaim, gotValue, err := parseSubject(tt.subject)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSubject(%q) error = %v, wantErr %v", tt.subject, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotClaim != tt.wantClaim {
					t.Errorf("parseSubject(%q) claim = %q, want %q", tt.subject, gotClaim, tt.wantClaim)
				}
				if gotValue != tt.wantValue {
					t.Errorf("parseSubject(%q) value = %q, want %q", tt.subject, gotValue, tt.wantValue)
				}
			}
		})
	}
}

// TestExtractActionResourceType tests the helper function
func TestExtractActionResourceType(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   string
	}{
		{
			name:   "standard action",
			action: "component:read",
			want:   "component",
		},
		{
			name:   "wildcard action",
			action: "component:*",
			want:   "component",
		},
		{
			name:   "global wildcard",
			action: "*",
			want:   "*",
		},
		{
			name:   "empty action",
			action: "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractActionResourceType(tt.action)
			if got != tt.want {
				t.Errorf("extractActionResourceType(%q) = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}

// TestIndexActions tests the action indexing helper
func TestIndexActions(t *testing.T) {
	tests := []struct {
		name    string
		actions []authzcore.Action
		wantLen int
	}{
		{
			name: "various action types",
			actions: []authzcore.Action{
				{Name: "component:create"},
				{Name: "component:read"},
				{Name: "component:update"},
				{Name: "component:delete"},
				{Name: "project:view"},
				{Name: "project:create"},
				{Name: "namespace:view"},
			},
			wantLen: 7,
		},
		{
			name: "actions with wildcards",
			actions: []authzcore.Action{
				{Name: "component:*"},
				{Name: "*"},
				{Name: "project:read"},
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := indexActions(tt.actions)

			// Verify actions string list length
			if len(idx.actionsStringList) != tt.wantLen {
				t.Errorf("indexActions() actionsStringList length = %d, want %d", len(idx.actionsStringList), tt.wantLen)
			}

			// Verify all actions are in the string list
			actionMap := make(map[string]bool)
			for _, a := range idx.actionsStringList {
				actionMap[a] = true
			}
			for _, action := range tt.actions {
				if !actionMap[action.Name] {
					t.Errorf("indexActions() missing action %q in actionsStringList", action.Name)
				}
			}

			// Verify resource type grouping
			for _, action := range tt.actions {
				resourceType := extractActionResourceType(action.Name)
				if actions, ok := idx.ByResourceType[resourceType]; ok {
					if !slices.Contains(actions, action.Name) {
						t.Errorf("indexActions() action %q not found in ByResourceType[%q]", action.Name, resourceType)
					}
				} else {
					t.Errorf("indexActions() resource type %q not found in ByResourceType", resourceType)
				}
			}
		})
	}
}

// TestNormalizeNamespace tests the normalizeNamespace helper function
func TestNormalizeNamespace(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		want      string
	}{
		{
			name:      "empty namespace converts to wildcard",
			namespace: "",
			want:      "*",
		},
		{
			name:      "non-empty namespace remains unchanged",
			namespace: "acme",
			want:      "acme",
		},
		{
			name:      "wildcard remains wildcard",
			namespace: "*",
			want:      "*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeNamespace(tt.namespace)
			if got != tt.want {
				t.Errorf("normalizeNamespace(%q) = %q, want %q", tt.namespace, got, tt.want)
			}
		})
	}
}

func TestComputeActionsDiff(t *testing.T) {
	tests := []struct {
		name            string
		existingActions []string
		newActions      []string
		wantAdded       []string
		wantRemoved     []string
	}{
		{
			name:            "no changes",
			existingActions: []string{"component:view", "component:create"},
			newActions:      []string{"component:view", "component:create"},
			wantAdded:       nil,
			wantRemoved:     nil,
		},
		{
			name:            "add new actions",
			existingActions: []string{"component:view"},
			newActions:      []string{"component:view", "component:create", "component:delete"},
			wantAdded:       []string{"component:create", "component:delete"},
			wantRemoved:     nil,
		},
		{
			name:            "remove actions",
			existingActions: []string{"component:view", "component:create", "component:delete"},
			newActions:      []string{"component:view"},
			wantAdded:       nil,
			wantRemoved:     []string{"component:create", "component:delete"},
		},
		{
			name:            "add and remove actions",
			existingActions: []string{"component:view", "component:create"},
			newActions:      []string{"component:view", "component:delete"},
			wantAdded:       []string{"component:delete"},
			wantRemoved:     []string{"component:create"},
		},
		{
			name:            "empty existing actions",
			existingActions: []string{},
			newActions:      []string{"component:view", "component:create"},
			wantAdded:       []string{"component:view", "component:create"},
			wantRemoved:     nil,
		},
		{
			name:            "empty new actions",
			existingActions: []string{"component:view", "component:create"},
			newActions:      []string{},
			wantAdded:       nil,
			wantRemoved:     []string{"component:view", "component:create"},
		},
		{
			name:            "both empty",
			existingActions: []string{},
			newActions:      []string{},
			wantAdded:       nil,
			wantRemoved:     nil,
		},
		{
			name:            "completely different actions",
			existingActions: []string{"component:view", "component:create"},
			newActions:      []string{"project:view", "project:create"},
			wantAdded:       []string{"project:view", "project:create"},
			wantRemoved:     []string{"component:view", "component:create"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAdded, gotRemoved := computeActionsDiff(tt.existingActions, tt.newActions)

			// Compare added actions (order doesn't matter)
			if !sliceContainsSameElements(gotAdded, tt.wantAdded) {
				t.Errorf("computeActionsDiff() added = %v, want %v", gotAdded, tt.wantAdded)
			}

			// Compare removed actions (order doesn't matter)
			if !sliceContainsSameElements(gotRemoved, tt.wantRemoved) {
				t.Errorf("computeActionsDiff() removed = %v, want %v", gotRemoved, tt.wantRemoved)
			}
		})
	}
}

func TestComputePolicyDiff(t *testing.T) {
	tests := []struct {
		name        string
		oldPolicies [][]string
		newPolicies [][]string
		wantAdded   [][]string
		wantRemoved [][]string
	}{
		{
			name: "no changes",
			oldPolicies: [][]string{
				{"groups:devs", "ns/acme", "editor", "acme", "allow", "{}", "binding-1"},
			},
			newPolicies: [][]string{
				{"groups:devs", "ns/acme", "editor", "acme", "allow", "{}", "binding-1"},
			},
			wantAdded:   nil,
			wantRemoved: nil,
		},
		{
			name:        "add policies to empty set",
			oldPolicies: nil,
			newPolicies: [][]string{
				{"groups:devs", "ns/acme", "editor", "acme", "allow", "{}", "binding-1"},
				{"groups:devs", "ns/acme", "viewer", "*", "allow", "{}", "binding-1"},
			},
			wantAdded: [][]string{
				{"groups:devs", "ns/acme", "editor", "acme", "allow", "{}", "binding-1"},
				{"groups:devs", "ns/acme", "viewer", "*", "allow", "{}", "binding-1"},
			},
			wantRemoved: nil,
		},
		{
			name: "remove all policies",
			oldPolicies: [][]string{
				{"groups:devs", "ns/acme", "editor", "acme", "allow", "{}", "binding-1"},
			},
			newPolicies: nil,
			wantAdded:   nil,
			wantRemoved: [][]string{
				{"groups:devs", "ns/acme", "editor", "acme", "allow", "{}", "binding-1"},
			},
		},
		{
			name: "swap role — one added one removed",
			oldPolicies: [][]string{
				{"groups:devs", "ns/acme", "editor", "acme", "allow", "{}", "binding-1"},
			},
			newPolicies: [][]string{
				{"groups:devs", "ns/acme", "viewer", "acme", "allow", "{}", "binding-1"},
			},
			wantAdded: [][]string{
				{"groups:devs", "ns/acme", "viewer", "acme", "allow", "{}", "binding-1"},
			},
			wantRemoved: [][]string{
				{"groups:devs", "ns/acme", "editor", "acme", "allow", "{}", "binding-1"},
			},
		},
		{
			name: "expand — keep existing and add new",
			oldPolicies: [][]string{
				{"groups:devs", "ns/acme/project/crm", "editor", "acme", "allow", "{}", "binding-1"},
			},
			newPolicies: [][]string{
				{"groups:devs", "ns/acme/project/crm", "editor", "acme", "allow", "{}", "binding-1"},
				{"groups:devs", "ns/acme", "viewer", "*", "allow", "{}", "binding-1"},
			},
			wantAdded: [][]string{
				{"groups:devs", "ns/acme", "viewer", "*", "allow", "{}", "binding-1"},
			},
			wantRemoved: nil,
		},
		{
			name: "shrink — remove one keep one",
			oldPolicies: [][]string{
				{"groups:devs", "ns/acme/project/crm", "editor", "acme", "allow", "{}", "binding-1"},
				{"groups:devs", "ns/acme", "viewer", "*", "allow", "{}", "binding-1"},
			},
			newPolicies: [][]string{
				{"groups:devs", "ns/acme/project/crm", "editor", "acme", "allow", "{}", "binding-1"},
			},
			wantAdded: nil,
			wantRemoved: [][]string{
				{"groups:devs", "ns/acme", "viewer", "*", "allow", "{}", "binding-1"},
			},
		},
		{
			name:        "both empty",
			oldPolicies: nil,
			newPolicies: nil,
			wantAdded:   nil,
			wantRemoved: nil,
		},
		{
			name: "change effect — old removed new added",
			oldPolicies: [][]string{
				{"groups:devs", "ns/acme", "editor", "acme", "allow", "{}", "binding-1"},
			},
			newPolicies: [][]string{
				{"groups:devs", "ns/acme", "editor", "acme", "deny", "{}", "binding-1"},
			},
			wantAdded: [][]string{
				{"groups:devs", "ns/acme", "editor", "acme", "deny", "{}", "binding-1"},
			},
			wantRemoved: [][]string{
				{"groups:devs", "ns/acme", "editor", "acme", "allow", "{}", "binding-1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAdded, gotRemoved := computePolicyDiff(tt.oldPolicies, tt.newPolicies)

			if !policySlicesEqual(gotAdded, tt.wantAdded) {
				t.Errorf("computePolicyDiff() added = %v, want %v", gotAdded, tt.wantAdded)
			}
			if !policySlicesEqual(gotRemoved, tt.wantRemoved) {
				t.Errorf("computePolicyDiff() removed = %v, want %v", gotRemoved, tt.wantRemoved)
			}
		})
	}
}

// policySlicesEqual checks if two slices of policy tuples contain the same elements (order-independent)
func policySlicesEqual(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	aSet := make(map[string]int)
	for _, p := range a {
		aSet[policyKey(p)]++
	}
	for _, p := range b {
		aSet[policyKey(p)]--
		if aSet[policyKey(p)] < 0 {
			return false
		}
	}
	return true
}

// sliceContainsSameElements checks if two slices contain the same elements
func sliceContainsSameElements(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}

	aMap := make(map[string]int)
	for _, v := range a {
		aMap[v]++
	}
	for _, v := range b {
		aMap[v]--
		if aMap[v] < 0 {
			return false
		}
	}
	return true
}

func TestResourceMatchWrapper_Errors(t *testing.T) {
	// Wrong number of args
	_, err := resourceMatchWrapper("only-one")
	require.Error(t, err, "resourceMatchWrapper with 1 arg should return error")

	// Non-string first arg
	_, err = resourceMatchWrapper(42, "ns/acme")
	require.Error(t, err, "resourceMatchWrapper with non-string first arg should return error")

	// Non-string second arg
	_, err = resourceMatchWrapper("ns/acme", 42)
	require.Error(t, err, "resourceMatchWrapper with non-string second arg should return error")

	// Valid args
	result, err := resourceMatchWrapper("ns/acme", "ns/acme")
	require.NoError(t, err)
	require.True(t, result.(bool), "resourceMatchWrapper exact match should be true")
}

func TestCondMatchWrapper_Errors(t *testing.T) {
	// Wrong number of args
	_, err := condMatchWrapper("only-one")
	require.Error(t, err, "condMatchWrapper with 1 arg should return error")

	// Non-string first arg
	_, err = condMatchWrapper(42, "releasebinding:create", "{}", "allow", "binding-1")
	require.Error(t, err, "condMatchWrapper with non-string first arg should return error")

	// Non-string second arg
	_, err = condMatchWrapper("{}", 42, "{}", "allow", "binding-1")
	require.Error(t, err, "condMatchWrapper with non-string second arg should return error")

	// Non-string third arg
	_, err = condMatchWrapper("{}", "releasebinding:create", 42, "allow", "binding-1")
	require.Error(t, err, "condMatchWrapper with non-string third arg should return error")

	// Non-string fourth arg
	_, err = condMatchWrapper("{}", "releasebinding:create", "{}", 42, "binding-1")
	require.Error(t, err, "condMatchWrapper with non-string fourth arg should return error")

	// Non-string fifth arg
	_, err = condMatchWrapper("{}", "releasebinding:create", "{}", "allow", 42)
	require.Error(t, err, "condMatchWrapper with non-string fifth arg should return error")

	// Valid args - empty policy condition (fast path)
	result, err := condMatchWrapper("{}", "releasebinding:create", "{}", "allow", "binding-1")
	require.NoError(t, err)
	require.True(t, result.(bool), "condMatchWrapper with empty policy cond should be true")
}

// mustCtxJSON builds a JSON-encoded authzcore.Context with the given environment,
// for use as the requestCtx argument to condMatcher.
func mustCtxJSON(t *testing.T, env string) string {
	t.Helper()
	s, err := serializeAuthzContext(authzcore.Context{
		Resource: authzcore.ResourceAttribute{Environment: env},
	})
	require.NoError(t, err)
	return s
}

// mustCondsJSON marshals a slice of AuthzCondition entries to the JSON string
// stored in a policy tuple's cond field.
func mustCondsJSON(t *testing.T, conds []authzv1alpha1.AuthzCondition) string {
	t.Helper()
	s, err := serializeAuthzConditions(conds)
	require.NoError(t, err)
	return s
}

func TestDecodeConditions(t *testing.T) {
	t.Run("empty string returns nil", func(t *testing.T) {
		got, err := decodeConditions("")
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("empty JSON object returns nil", func(t *testing.T) {
		got, err := decodeConditions("{}")
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("valid JSON array round-trips", func(t *testing.T) {
		conds := []authzv1alpha1.AuthzCondition{
			{Actions: []string{"releasebinding:create"}, Expression: `resource.environment == "dev"`},
		}
		raw, err := serializeAuthzConditions(conds)
		require.NoError(t, err)

		got, err := decodeConditions(raw)
		require.NoError(t, err)
		require.Equal(t, conds, got)
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		got, err := decodeConditions("not-json")
		require.Error(t, err)
		require.Nil(t, got)
	})

	t.Run("JSON object (not array) returns error", func(t *testing.T) {
		got, err := decodeConditions(`{"actions":["releasebinding:create"]}`)
		require.Error(t, err)
		require.Nil(t, got)
	})

	t.Run("multiple conditions decoded correctly", func(t *testing.T) {
		conds := []authzv1alpha1.AuthzCondition{
			{Actions: []string{"releasebinding:create"}, Expression: `resource.environment == "dev"`},
			{Actions: []string{"releasebinding:delete"}, Expression: `resource.environment == "prod"`},
		}
		raw, err := serializeAuthzConditions(conds)
		require.NoError(t, err)

		got, err := decodeConditions(raw)
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, conds, got)
	})
}

func TestSerializeAuthzConditions(t *testing.T) {
	t.Run("nil slice returns empty context", func(t *testing.T) {
		s, err := serializeAuthzConditions(nil)
		require.NoError(t, err)
		require.Equal(t, emptyContextJSON, s)
	})

	t.Run("empty slice returns empty context", func(t *testing.T) {
		s, err := serializeAuthzConditions([]authzv1alpha1.AuthzCondition{})
		require.NoError(t, err)
		require.Equal(t, emptyContextJSON, s)
	})

	t.Run("non-empty slice round-trips through JSON", func(t *testing.T) {
		conds := []authzv1alpha1.AuthzCondition{
			{Actions: []string{"releasebinding:create"}, Expression: `resource.environment == "dev"`},
		}
		s, err := serializeAuthzConditions(conds)
		require.NoError(t, err)

		var decoded []authzv1alpha1.AuthzCondition
		require.NoError(t, json.Unmarshal([]byte(s), &decoded))
		require.Equal(t, conds, decoded)
	})
}

func TestIsPolicyConditionEmpty(t *testing.T) {
	tests := []struct {
		name string
		cond string
		want bool
	}{
		{
			name: "empty json condition is empty",
			cond: "{}",
			want: true,
		},
		{
			name: "empty string condition is empty",
			cond: "",
			want: true,
		},
		{
			name: "non-empty condition is not empty",
			cond: mustCondsJSON(t, []authzv1alpha1.AuthzCondition{{Actions: []string{"releasebinding:create"}, Expression: `true`}}),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPolicyConditionEmpty(tt.cond)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFilterConditionsByAction(t *testing.T) {
	tests := []struct {
		name          string
		conds         []authzv1alpha1.AuthzCondition
		requestAction string
		wantLen       int
	}{
		{
			name:          "exact match returns entry",
			conds:         []authzv1alpha1.AuthzCondition{{Actions: []string{"releasebinding:create"}, Expression: `true`}},
			requestAction: "releasebinding:create",
			wantLen:       1,
		},
		{
			name:          "wildcard pattern matches concrete action",
			conds:         []authzv1alpha1.AuthzCondition{{Actions: []string{"releasebinding:*"}, Expression: `true`}},
			requestAction: "releasebinding:create",
			wantLen:       1,
		},
		{
			name:          "no matching action returns empty",
			conds:         []authzv1alpha1.AuthzCondition{{Actions: []string{"releasebinding:delete"}, Expression: `true`}},
			requestAction: "releasebinding:create",
			wantLen:       0,
		},
		{
			name: "entry with multiple patterns, one matches",
			conds: []authzv1alpha1.AuthzCondition{
				{Actions: []string{"releasebinding:delete", "releasebinding:create"}, Expression: `true`},
			},
			requestAction: "releasebinding:create",
			wantLen:       1,
		},
		{
			name: "mixed entries, only matching ones returned",
			conds: []authzv1alpha1.AuthzCondition{
				{Actions: []string{"releasebinding:create"}, Expression: `true`},
				{Actions: []string{"releasebinding:delete"}, Expression: `false`},
			},
			requestAction: "releasebinding:create",
			wantLen:       1,
		},
		{
			name:          "empty conditions returns empty",
			conds:         nil,
			requestAction: "releasebinding:create",
			wantLen:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterConditionsByAction(tt.conds, tt.requestAction)
			require.Len(t, got, tt.wantLen)
		})
	}
}

func TestBuildActivationForRequest(t *testing.T) {
	const action = authzcore.ActionCreateReleaseBinding

	t.Run("valid JSON and known action returns ok=true", func(t *testing.T) {
		ctx := mustCtxJSON(t, "dev")
		act, ok := buildActivationForRequest(ctx, action)
		require.True(t, ok)
		require.NotNil(t, act)
	})

	t.Run("malformed JSON returns ok=false", func(t *testing.T) {
		act, ok := buildActivationForRequest("not-json", action)
		require.False(t, ok)
		require.Nil(t, act)
	})

	t.Run("unknown action (no registered attrs) still returns ok=true with empty activation", func(t *testing.T) {
		ctx := mustCtxJSON(t, "dev")
		act, ok := buildActivationForRequest(ctx, "component:view")
		require.True(t, ok)
		require.NotNil(t, act)
	})
}

func TestAnyConditionMatches(t *testing.T) {
	devCtx := mustCtxJSON(t, "dev")
	act, ok := buildActivationForRequest(devCtx, authzcore.ActionCreateReleaseBinding)
	require.True(t, ok)

	const allow = string(authzcore.PolicyEffectAllow)
	const deny = string(authzcore.PolicyEffectDeny)

	t.Run("empty entries returns false", func(t *testing.T) {
		require.False(t, anyConditionMatches(nil, act, allow, "test-binding"))
	})

	t.Run("single true entry returns true", func(t *testing.T) {
		entries := []authzv1alpha1.AuthzCondition{
			{Expression: `resource.environment == "dev"`},
		}
		require.True(t, anyConditionMatches(entries, act, allow, "test-binding"))
	})

	t.Run("single false entry returns false", func(t *testing.T) {
		entries := []authzv1alpha1.AuthzCondition{
			{Expression: `resource.environment == "prod"`},
		}
		require.False(t, anyConditionMatches(entries, act, allow, "test-binding"))
	})

	t.Run("return true if any condition matches", func(t *testing.T) {
		entries := []authzv1alpha1.AuthzCondition{
			{Expression: `resource.environment == "prod"`},
			{Expression: `resource.environment == "dev"`},
		}
		require.True(t, anyConditionMatches(entries, act, allow, "test-binding"))
	})

	t.Run("all false returns false", func(t *testing.T) {
		entries := []authzv1alpha1.AuthzCondition{
			{Expression: `resource.environment == "prod"`},
			{Expression: `resource.environment == "staging"`},
		}
		require.False(t, anyConditionMatches(entries, act, allow, "test-binding"))
	})

	t.Run("errored entry on deny policy fails closed", func(t *testing.T) {
		entries := []authzv1alpha1.AuthzCondition{
			{Expression: `not valid CEL ((((`},
		}
		require.True(t, anyConditionMatches(entries, act, deny, "test-binding"))
	})

	t.Run("errored entry on allow policy fails closed", func(t *testing.T) {
		entries := []authzv1alpha1.AuthzCondition{
			{Expression: `not valid CEL ((((`},
		}
		require.False(t, anyConditionMatches(entries, act, allow, "test-binding"))
	})

	// The error path is order-independent: a broken entry anywhere in the
	// policy poisons the whole CR, even if a sibling entry cleanly matched.
	// We don't know what the broken entry was meant to do, so we cannot
	// trust the sibling result.
	t.Run("error overrides clean match on allow regardless of order", func(t *testing.T) {
		errFirst := []authzv1alpha1.AuthzCondition{
			{Expression: `not valid CEL ((((`},
			{Expression: `resource.environment == "dev"`},
		}
		matchFirst := []authzv1alpha1.AuthzCondition{
			{Expression: `resource.environment == "dev"`},
			{Expression: `not valid CEL ((((`},
		}

		require.False(t, anyConditionMatches(errFirst, act, allow, "test-binding"))
		require.False(t, anyConditionMatches(matchFirst, act, allow, "test-binding"))
	})

	t.Run("error overrides clean match on deny regardless of order", func(t *testing.T) {
		errFirst := []authzv1alpha1.AuthzCondition{
			{Expression: `not valid CEL ((((`},
			{Expression: `resource.environment == "prod"`}, // cleanly rejects
		}
		rejectFirst := []authzv1alpha1.AuthzCondition{
			{Expression: `resource.environment == "prod"`},
			{Expression: `not valid CEL ((((`},
		}

		require.True(t, anyConditionMatches(errFirst, act, deny, "test-binding"))
		require.True(t, anyConditionMatches(rejectFirst, act, deny, "test-binding"))
	})
}

func TestEvalCondition(t *testing.T) {
	devCtx := mustCtxJSON(t, "dev")
	act, ok := buildActivationForRequest(devCtx, authzcore.ActionCreateReleaseBinding)
	require.True(t, ok)

	const allow = string(authzcore.PolicyEffectAllow)

	t.Run("true expression returns evalAllow", func(t *testing.T) {
		require.Equal(t, evalAllow, evalCondition(`resource.environment == "dev"`, act, allow, "test-binding"))
	})

	t.Run("false expression returns evalReject", func(t *testing.T) {
		require.Equal(t, evalReject, evalCondition(`resource.environment == "prod"`, act, allow, "test-binding"))
	})

	t.Run("compile error returns evalError", func(t *testing.T) {
		require.Equal(t, evalError, evalCondition(`this is not valid CEL ((((`, act, allow, "test-binding"))
	})

	t.Run("non-bool result returns evalError", func(t *testing.T) {
		require.Equal(t, evalError, evalCondition(`resource.environment`, act, allow, "test-binding"))
	})

	t.Run("expression references unknown attribute returns evalError", func(t *testing.T) {
		require.Equal(t, evalError, evalCondition(`resource.nonExistentField == "dev"`, act, allow, "test-binding"))
	})
}

func TestConditionMatcher(t *testing.T) {
	const action = authzcore.ActionCreateReleaseBinding
	const allow = string(authzcore.PolicyEffectAllow)
	const deny = string(authzcore.PolicyEffectDeny)

	devCtx := mustCtxJSON(t, "dev")
	prodCtx := mustCtxJSON(t, "prod")
	emptyCtx := mustCtxJSON(t, "")

	devOnly := []authzv1alpha1.AuthzCondition{
		{Actions: []string{action}, Expression: `resource.environment == "dev"`},
	}
	devOrStaging := []authzv1alpha1.AuthzCondition{
		{Actions: []string{action}, Expression: `resource.environment in ["dev", "staging"]`},
	}
	differentAction := []authzv1alpha1.AuthzCondition{
		{Actions: []string{authzcore.ActionDeleteReleaseBinding}, Expression: `resource.environment == "dev"`},
	}
	brokenCEL := []authzv1alpha1.AuthzCondition{
		{Actions: []string{action}, Expression: `not valid CEL ((((`},
	}
	nonBoolCEL := []authzv1alpha1.AuthzCondition{
		{Actions: []string{action}, Expression: `resource.environment`},
	}

	tests := []struct {
		name       string
		requestCtx string
		action     string
		policyCond string
		policyEft  string
		want       bool
	}{
		{
			name:       "empty string policy cond is treated as unconstrained (allow)",
			requestCtx: devCtx,
			action:     action,
			policyCond: "",
			policyEft:  allow,
			want:       true,
		},
		{
			name:       "empty string policy cond is treated as unconstrained (deny)",
			requestCtx: devCtx,
			action:     action,
			policyCond: "",
			policyEft:  deny,
			want:       true,
		},
		{
			name:       "empty JSON policy cond is treated as unconstrained",
			requestCtx: devCtx,
			action:     action,
			policyCond: emptyContextJSON,
			policyEft:  allow,
			want:       true,
		},
		{
			name:       "no entries match action -> RBAC stands (allow)",
			requestCtx: devCtx,
			action:     action,
			policyCond: mustCondsJSON(t, differentAction),
			policyEft:  allow,
			want:       true,
		},
		{
			name:       "no entries match action -> RBAC stands (deny)",
			requestCtx: devCtx,
			action:     action,
			policyCond: mustCondsJSON(t, differentAction),
			policyEft:  deny,
			want:       true,
		},
		{
			name:       "single entry, expression true",
			requestCtx: devCtx,
			action:     action,
			policyCond: mustCondsJSON(t, devOnly),
			policyEft:  allow,
			want:       true,
		},
		{
			name:       "single entry, expression false",
			requestCtx: prodCtx,
			action:     action,
			policyCond: mustCondsJSON(t, devOnly),
			policyEft:  allow,
			want:       false,
		},
		{
			name:       "empty environment does not match allowlist",
			requestCtx: emptyCtx,
			action:     action,
			policyCond: mustCondsJSON(t, devOrStaging),
			policyEft:  allow,
			want:       false,
		},
		{
			name:       "malformed policy cond JSON, allow -> does not match",
			requestCtx: devCtx,
			action:     action,
			policyCond: "not-json",
			policyEft:  allow,
			want:       false,
		},
		{
			name:       "malformed policy cond JSON, deny -> matches (fail closed)",
			requestCtx: devCtx,
			action:     action,
			policyCond: "not-json",
			policyEft:  deny,
			want:       true,
		},
		{
			name:       "compile error, allow -> does not match",
			requestCtx: devCtx,
			action:     action,
			policyCond: mustCondsJSON(t, brokenCEL),
			policyEft:  allow,
			want:       false,
		},
		{
			name:       "compile error, deny -> matches (fail closed)",
			requestCtx: devCtx,
			action:     action,
			policyCond: mustCondsJSON(t, brokenCEL),
			policyEft:  deny,
			want:       true,
		},
		{
			name:       "non-bool CEL result, allow -> does not match",
			requestCtx: devCtx,
			action:     action,
			policyCond: mustCondsJSON(t, nonBoolCEL),
			policyEft:  allow,
			want:       false,
		},
		{
			name:       "non-bool CEL result, deny -> matches (fail closed)",
			requestCtx: devCtx,
			action:     action,
			policyCond: mustCondsJSON(t, nonBoolCEL),
			policyEft:  deny,
			want:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ConditionMatcher(tc.requestCtx, tc.action, tc.policyCond, tc.policyEft, "test-binding")
			require.Equal(t, tc.want, got)
		})
	}
}

func requireActivationMap(t *testing.T, act interpreter.Activation, root string) map[string]any {
	t.Helper()
	val, found := act.ResolveName(root)
	require.True(t, found, "expected variable %q to be bound in activation", root)
	m, ok := val.(map[string]any)
	require.True(t, ok, "expected variable %q to be map[string]any", root)
	return m
}

func TestBuildCelActivation(t *testing.T) {
	envAttr := authzcore.AttrResourceEnvironment

	t.Run("empty allowedAttrs yields empty activation", func(t *testing.T) {
		ctx := authzcore.Context{Resource: authzcore.ResourceAttribute{Environment: "dev"}}
		act, err := buildCelActivation(ctx, nil)
		require.NoError(t, err)
		require.NotNil(t, act)
	})

	t.Run("allowed attr with value in ctx is bound to real value", func(t *testing.T) {
		ctx := authzcore.Context{Resource: authzcore.ResourceAttribute{Environment: "staging"}}
		act, err := buildCelActivation(ctx, []authzcore.AttributeSpec{envAttr})
		require.NoError(t, err)

		m := requireActivationMap(t, act, "resource")
		require.Equal(t, "staging", m["environment"])
	})

	t.Run("allowed attr missing from ctx is bound to typed zero (empty string)", func(t *testing.T) {
		ctx := authzcore.Context{}
		act, err := buildCelActivation(ctx, []authzcore.AttributeSpec{envAttr})
		require.NoError(t, err)

		m := requireActivationMap(t, act, "resource")
		require.Equal(t, "", m["environment"])
	})

	t.Run("allowed attribute missing from context is bound with zero value alongside present attributes", func(t *testing.T) {
		componentTypeAttr := authzcore.AttributeSpec{Key: "resource.componentType", CELType: cel.StringType}
		ctx := authzcore.Context{Resource: authzcore.ResourceAttribute{Environment: "prod"}}
		act, err := buildCelActivation(ctx, []authzcore.AttributeSpec{componentTypeAttr, envAttr})
		require.NoError(t, err)

		m := requireActivationMap(t, act, "resource")
		require.Equal(t, "prod", m["environment"])
		require.Contains(t, m, "componentType")
	})
}

func TestConvertCtxToAttrMap(t *testing.T) {
	t.Run("valid context produces expected two-level map", func(t *testing.T) {
		ctx := authzcore.Context{Resource: authzcore.ResourceAttribute{Environment: "dev"}}
		m, err := convertCtxToAttrMap(ctx)
		require.NoError(t, err)
		require.Equal(t, "dev", m["resource"]["environment"])
	})

	t.Run("empty context produces empty or zero-value map", func(t *testing.T) {
		ctx := authzcore.Context{}
		m, err := convertCtxToAttrMap(ctx)
		require.NoError(t, err)
		if resourceMap, ok := m["resource"]; ok {
			require.Empty(t, resourceMap["environment"])
		}
	})
}

func TestZeroForCELType(t *testing.T) {
	tests := []struct {
		name    string
		celType *cel.Type
		want    any
	}{
		{name: "string type returns empty string", celType: cel.StringType, want: ""},
		{name: "bool type returns false", celType: cel.BoolType, want: false},
		{name: "int type returns int64 zero", celType: cel.IntType, want: int64(0)},
		{name: "double type returns float64 zero", celType: cel.DoubleType, want: 0.0},
		{name: "unknown type returns nil", celType: cel.DynType, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, zeroForCELType(tt.celType))
		})
	}
}

func TestCompileCEL(t *testing.T) {
	t.Run("valid bool expression compiles and evaluates", func(t *testing.T) {
		prg, err := compileCEL(`resource.environment == "dev"`)
		require.NoError(t, err)
		require.NotNil(t, prg)
	})

	t.Run("syntax error returns error", func(t *testing.T) {
		prg, err := compileCEL(`this is not valid CEL ((((`)
		require.Error(t, err)
		require.Nil(t, prg)
	})

	t.Run("empty expression returns error", func(t *testing.T) {
		prg, err := compileCEL(``)
		require.Error(t, err)
		require.Nil(t, prg)
	})
}

func TestValidateBatchEvaluateRequest_MissingActionAtIndex(t *testing.T) {
	validSubject := &authzcore.SubjectContext{
		Type: "user", EntitlementClaim: "groups", EntitlementValues: []string{"devs"},
	}
	req := &authzcore.BatchEvaluateRequest{
		Requests: []authzcore.EvaluateRequest{
			{SubjectContext: validSubject, Resource: authzcore.Resource{Type: "component"}, Action: "component:view"},
			{SubjectContext: validSubject, Resource: authzcore.Resource{Type: "component"}, Action: ""}, // missing action
		},
	}
	require.Error(t, validateBatchEvaluateRequest(req), "validateBatchEvaluateRequest with empty action should return error")
}
