// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"

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

func TestPopulateSubjectClaims(t *testing.T) {
	// Helper to create JWT token with claims
	createJWT := func(claims jwt.MapClaims) string {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenString, _ := token.SignedString([]byte("test-secret"))
		return tokenString
	}

	tests := []struct {
		name    string
		subject *authzcore.Subject
		want    *authzcore.SubjectContext
		wantErr bool
	}{
		{
			name: "single group claim - string",
			subject: &authzcore.Subject{
				JwtToken: createJWT(jwt.MapClaims{
					"group": "admin-group",
				}),
			},
			want: &authzcore.SubjectContext{
				Type:              authzcore.SubjectTypeUser,
				EntitlementValues: []string{"admin-group"},
			},
			wantErr: false,
		},
		{
			name: "multiple groups claim - array",
			subject: &authzcore.Subject{
				JwtToken: createJWT(jwt.MapClaims{
					"group": []interface{}{"admin-group", "dev-group"},
				}),
			},
			want: &authzcore.SubjectContext{
				Type:              authzcore.SubjectTypeUser,
				EntitlementValues: []string{"admin-group", "dev-group"},
			},
			wantErr: false,
		},
		{
			name: "service account claim",
			subject: &authzcore.Subject{
				JwtToken: createJWT(jwt.MapClaims{
					"service_account": "api-service",
				}),
			},
			want: &authzcore.SubjectContext{
				Type:              authzcore.SubjectTypeServiceAccount,
				EntitlementValues: []string{"api-service"},
			},
			wantErr: false,
		},
		{
			name: "empty group string returns empty claims",
			subject: &authzcore.Subject{
				JwtToken: createJWT(jwt.MapClaims{
					"group": "",
				}),
			},
			want: &authzcore.SubjectContext{
				Type:              authzcore.SubjectTypeUser,
				EntitlementValues: []string{},
			},
			wantErr: false,
		},
		{
			name: "no valid claims",
			subject: &authzcore.Subject{
				JwtToken: createJWT(jwt.MapClaims{
					"other": "value",
				}),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "malformed JWT",
			subject: &authzcore.Subject{
				JwtToken: "not-a-valid-jwt",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty JWT token",
			subject: &authzcore.Subject{
				JwtToken: "",
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := populateSubjectClaims(tt.subject)

			if tt.wantErr {
				if err == nil {
					t.Errorf("populateSubjectClaims() error = nil, wantErr = true")
				}
				return
			}

			if err != nil {
				t.Errorf("populateSubjectClaims() unexpected error = %v", err)
				return
			}

			if got.Type != tt.want.Type {
				t.Errorf("Type = %v, want %v", got.Type, tt.want.Type)
			}

			if len(got.EntitlementValues) != len(tt.want.EntitlementValues) {
				t.Errorf("EntitlementValues length = %d, want %d", len(got.EntitlementValues), len(tt.want.EntitlementValues))
			} else {
				for i := range got.EntitlementValues {
					if got.EntitlementValues[i] != tt.want.EntitlementValues[i] {
						t.Errorf("EntitlementValues[%d] = %q, want %q", i, got.EntitlementValues[i], tt.want.EntitlementValues[i])
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
				Subject: authzcore.Subject{
					JwtToken: "valid-token",
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
			name: "missing JWT token",
			request: &authzcore.EvaluateRequest{
				Subject: authzcore.Subject{
					JwtToken: "",
				},
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
				Subject: authzcore.Subject{
					JwtToken: "valid-token",
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
				Subject: authzcore.Subject{
					JwtToken: "valid-token",
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
		Subject: authzcore.Subject{
			JwtToken: "valid-token",
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
			name: "invalid request at index 0 - missing JWT",
			request: &authzcore.BatchEvaluateRequest{
				Requests: []authzcore.EvaluateRequest{
					{
						Subject: authzcore.Subject{
							JwtToken: "",
						},
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
						Subject: authzcore.Subject{
							JwtToken: "valid-token",
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
