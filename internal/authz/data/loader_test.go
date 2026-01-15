// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package data

import (
	"os"
	"path/filepath"
	"testing"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

func TestLoadActions(t *testing.T) {
	actions, err := LoadActions()
	if err != nil {
		t.Fatalf("LoadActions() error = %v", err)
	}

	if len(actions) == 0 {
		t.Error("LoadActions() returned empty actions list")
	}

	// Verify some expected actions exist
	expectedActions := []string{
		"organization:view",
		"project:view",
		"project:create",
		"component:view",
		"component:create",
		"component:update",
	}

	for _, expected := range expectedActions {
		found := false
		for _, action := range actions {
			if action.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("LoadActions() missing expected action: %s", expected)
		}
	}
}

func TestLoadEmbeddedAuthzData(t *testing.T) {
	data, err := LoadEmbeddedAuthzData()
	if err != nil {
		t.Fatalf("LoadEmbeddedAuthzData() error = %v", err)
	}

	if len(data.Roles) == 0 {
		t.Error("LoadEmbeddedAuthzData() returned empty roles list")
	}
}

func TestLoadDefaultAuthzDataFromFile(t *testing.T) {
	tests := []struct {
		name             string
		setupFile        bool
		filePath         string
		fileContent      string
		wantErr          bool
		expectedRoles    int
		expectedMappings int
	}{
		{
			name:      "valid file with roles and mappings",
			setupFile: true,
			filePath:  "test_authz_data.yaml",
			fileContent: `roles:
  - name: admin
    actions:
      - "*"
mappings:
  - role:
      name: admin
    entitlement:
      claim: groups
      value: admin-group
    hierarchy:
      namespace: "acme"
    effect: allow
  - role:
      name: viewer
    entitlement:
      claim: groups
      value: viewers
    effect: allow
`,
			wantErr:          false,
			expectedRoles:    1,
			expectedMappings: 2,
		},
		{
			name:      "invalid file - mapping with missing required field",
			setupFile: true,
			filePath:  "test_authz_data.yaml",
			fileContent: `roles:
  - name: viewer
    actions:
      - "component:view"
mappings:
  - role:
      name: viewer
    entitlement:
      claim: groups
      value: viewers
`,
			wantErr: true,
		},
		{
			name:      "invalid file - mapping with missing role_ref",
			setupFile: true,
			filePath:  "test_authz_data.yaml",
			fileContent: `roles:
  - name: admin
    actions:
      - "*"
mappings:
  - entitlement:
      claim: groups
      value: admin-group
    effect: allow
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFilePath := tt.filePath
			if tt.setupFile {
				tmpDir := t.TempDir()
				testFilePath = filepath.Join(tmpDir, tt.filePath)
				if err := os.WriteFile(testFilePath, []byte(tt.fileContent), 0600); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
			}
			data, err := LoadDefaultAuthzDataFromFile(testFilePath)

			if (err != nil) != tt.wantErr {
				t.Errorf("LoadDefaultAuthzDataFromFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check expected counts if specified (only if no error)
			if !tt.wantErr {
				if tt.expectedRoles >= 0 && len(data.Roles) != tt.expectedRoles {
					t.Errorf("LoadDefaultAuthzDataFromFile() returned %d roles, want %d", len(data.Roles), tt.expectedRoles)
				}
				if tt.expectedMappings >= 0 && len(data.Mappings) != tt.expectedMappings {
					t.Errorf("LoadDefaultAuthzDataFromFile() returned %d mappings, want %d", len(data.Mappings), tt.expectedMappings)
				}
			}
		})
	}
}

func TestValidateRoles(t *testing.T) {
	tests := []struct {
		name    string
		roles   []authzcore.Role
		wantErr bool
	}{
		{
			name: "valid roles",
			roles: []authzcore.Role{
				{Name: "admin", Actions: []string{"*"}},
				{Name: "viewer", Actions: []string{"component:view"}},
			},
			wantErr: false,
		},
		{
			name:    "empty roles list",
			roles:   []authzcore.Role{},
			wantErr: true,
		},
		{
			name: "role with empty name",
			roles: []authzcore.Role{
				{Name: "", Actions: []string{"component:view"}},
			},
			wantErr: true,
		},
		{
			name: "role with empty actions",
			roles: []authzcore.Role{
				{Name: "test-role", Actions: []string{}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRoles(tt.roles)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRoles() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateMappings(t *testing.T) {
	tests := []struct {
		name     string
		mappings []authzcore.RoleEntitlementMapping
		wantErr  bool
	}{
		{
			name: "valid mapping",
			mappings: []authzcore.RoleEntitlementMapping{
				{
					RoleRef: authzcore.RoleRef{Name: "admin"},
					Entitlement: authzcore.Entitlement{
						Claim: "groups",
						Value: "admin-group",
					},
					Hierarchy: authzcore.ResourceHierarchy{
						Namespace: "acme",
						Project:   "payment",
					},
					Effect: authzcore.PolicyEffectAllow,
				},
			},
			wantErr: false,
		},
		{
			name: "empty role_ref name",
			mappings: []authzcore.RoleEntitlementMapping{
				{
					RoleRef: authzcore.RoleRef{Name: ""},
					Entitlement: authzcore.Entitlement{
						Claim: "groups",
						Value: "admin-group",
					},
					Effect: authzcore.PolicyEffectAllow,
				},
			},
			wantErr: true,
		},
		{
			name: "empty entitlement claim",
			mappings: []authzcore.RoleEntitlementMapping{
				{
					RoleRef: authzcore.RoleRef{Name: "admin"},
					Entitlement: authzcore.Entitlement{
						Claim: "",
						Value: "",
					},
					Effect: authzcore.PolicyEffectAllow,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid effect",
			mappings: []authzcore.RoleEntitlementMapping{
				{
					RoleRef: authzcore.RoleRef{Name: "admin"},
					Entitlement: authzcore.Entitlement{
						Claim: "groups",
						Value: "admin-group",
					},
					Effect: "invalid",
				},
			},
			wantErr: true,
		},
		{
			name: "multiple valid mappings",
			mappings: []authzcore.RoleEntitlementMapping{
				{
					RoleRef: authzcore.RoleRef{Name: "admin"},
					Entitlement: authzcore.Entitlement{
						Claim: "groups",
						Value: "admin-group",
					},
					Effect: authzcore.PolicyEffectAllow,
				},
				{
					RoleRef: authzcore.RoleRef{Name: "viewer"},
					Entitlement: authzcore.Entitlement{
						Claim: "groups",
						Value: "viewers",
					},
					Effect: authzcore.PolicyEffectAllow,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMappings(tt.mappings)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMappings() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
