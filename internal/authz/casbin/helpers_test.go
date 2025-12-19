// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"testing"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

func TestResourceMatch(t *testing.T) {
	tests := []struct {
		name            string
		requestResource string
		policyResource  string
		want            bool
	}{
		{
			name:            "exact match",
			requestResource: "org/acme",
			policyResource:  "org/acme",
			want:            true,
		},
		{
			name:            "hierarchical prefix match",
			requestResource: "org/acme/project/p1/component/c1",
			policyResource:  "org/acme/project/p1",
			want:            true,
		},
		{
			name:            "no match - different organization",
			requestResource: "org/other/project/p1",
			policyResource:  "org/acme",
			want:            false,
		},
		{
			name:            "no match - partial prefix without hierarchy boundary",
			requestResource: "org/acme-other",
			policyResource:  "org/acme",
			want:            false,
		},
		{
			name:            "wildcard matches any resource",
			requestResource: "org/acme/project/p1/component/c1",
			policyResource:  "*",
			want:            true,
		},
		{
			name:            "no match - policy is more specific than request",
			requestResource: "org/acme",
			policyResource:  "org/acme/project/p1",
			want:            false,
		},
		{
			name:            "empty request resource",
			requestResource: "",
			policyResource:  "org/acme",
			want:            false,
		},
		{
			name:            "empty policy resource",
			requestResource: "org/acme",
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
			name: "full hierarchy without organization units",
			hierarchy: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "p1",
				Component:    "c1",
			},
			want: "org/acme/project/p1/component/c1",
		},
		{
			name: "organization only",
			hierarchy: authzcore.ResourceHierarchy{
				Organization: "acme",
			},
			want: "org/acme",
		},
		{
			name: "full hierarchy with organization units",
			hierarchy: authzcore.ResourceHierarchy{
				Organization:      "acme",
				OrganizationUnits: []string{"sales", "emea"},
				Project:           "p1",
			},
			want: "org/acme/ou/sales/ou/emea/project/p1",
		},
		{
			name:      "empty hierarchy - returns wildcard",
			hierarchy: authzcore.ResourceHierarchy{},
			want:      "*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hierarchyToResourcePath(tt.hierarchy)
			if got != tt.want {
				t.Errorf("hierarchyToResourcePath() = %q, want %q", got, tt.want)
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
			name:         "full hierarchy path without organization units",
			resourcePath: "org/acme/project/p1/component/c1",
			want: authzcore.ResourceHierarchy{
				Organization: "acme",
				Project:      "p1",
				Component:    "c1",
			},
		},
		{
			name:         "full hierarchy path with organization units",
			resourcePath: "org/acme/ou/sales/ou/emea/project/p1/component/c1",
			want: authzcore.ResourceHierarchy{
				Organization:      "acme",
				OrganizationUnits: []string{"sales", "emea"},
				Project:           "p1",
				Component:         "c1",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resourcePathToHierarchy(tt.resourcePath)

			if got.Organization != tt.want.Organization {
				t.Errorf("Organization = %q, want %q", got.Organization, tt.want.Organization)
			}
			if got.Project != tt.want.Project {
				t.Errorf("Project = %q, want %q", got.Project, tt.want.Project)
			}
			if got.Component != tt.want.Component {
				t.Errorf("Component = %q, want %q", got.Component, tt.want.Component)
			}

			if len(got.OrganizationUnits) != len(tt.want.OrganizationUnits) {
				t.Errorf("OrganizationUnits length = %d, want %d", len(got.OrganizationUnits), len(tt.want.OrganizationUnits))
			} else {
				for i := range got.OrganizationUnits {
					if got.OrganizationUnits[i] != tt.want.OrganizationUnits[i] {
						t.Errorf("OrganizationUnits[%d] = %q, want %q", i, got.OrganizationUnits[i], tt.want.OrganizationUnits[i])
					}
				}
			}
		})
	}
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
					Type:              authzcore.SubjectTypeUser,
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"test-group"},
				},
				Scope: authzcore.ResourceHierarchy{
					Organization: "acme",
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
					Organization: "acme",
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
			policyResource: "org/acme",
			scopePath:      "org/acme",
			want:           true,
		},
		{
			name:           "policy is broader (parent) than scope",
			policyResource: "org/acme",
			scopePath:      "org/acme/project/p1",
			want:           true,
		},
		{
			name:           "policy is narrower (child) than scope",
			policyResource: "org/acme/project/p1",
			scopePath:      "org/acme",
			want:           true,
		},
		{
			name:           "wildcard policy matches any scope",
			policyResource: "*",
			scopePath:      "org/acme/project/p1",
			want:           true,
		},
		{
			name:           "wildcard scope matches any policy",
			policyResource: "org/acme/project/p1",
			scopePath:      "*",
			want:           true,
		},
		{
			name:           "different scopes - no match",
			policyResource: "org/acme/project/p1",
			scopePath:      "org/acme/project/p2",
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
	testActions := []Action{
		{Action: "component:create"},
		{Action: "component:view"},
		{Action: "component:delete"},
		{Action: "project:view"},
		{Action: "project:update"},
		{Action: "organization:view"},
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
			want:          []string{"component:create", "component:view", "component:delete", "project:view", "project:update", "organization:view"},
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
		actions []Action
		wantLen int
	}{
		{
			name: "various action types",
			actions: []Action{
				{Action: "component:create"},
				{Action: "component:read"},
				{Action: "component:update"},
				{Action: "component:delete"},
				{Action: "project:view"},
				{Action: "project:create"},
				{Action: "organization:view"},
			},
			wantLen: 7,
		},
		{
			name: "actions with wildcards",
			actions: []Action{
				{Action: "component:*"},
				{Action: "*"},
				{Action: "project:read"},
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
				if !actionMap[action.Action] {
					t.Errorf("indexActions() missing action %q in actionsStringList", action.Action)
				}
			}

			// Verify resource type grouping
			for _, action := range tt.actions {
				resourceType := extractActionResourceType(action.Action)
				if actions, ok := idx.ByResourceType[resourceType]; ok {
					found := false
					for _, a := range actions {
						if a == action.Action {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("indexActions() action %q not found in ByResourceType[%q]", action.Action, resourceType)
					}
				} else {
					t.Errorf("indexActions() resource type %q not found in ByResourceType", resourceType)
				}
			}
		})
	}
}
