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
			if action == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("LoadActions() missing expected action: %s", expected)
		}
	}
}

func TestLoadEmbeddedRoles(t *testing.T) {
	roles, err := LoadEmbeddedRoles()
	if err != nil {
		t.Fatalf("LoadEmbeddedRoles() error = %v", err)
	}

	if len(roles) == 0 {
		t.Error("LoadEmbeddedRoles() returned empty roles list")
	}
}

func TestLoadRolesFromFile(t *testing.T) {
	tests := []struct {
		name          string
		setupFile     bool
		filePath      string
		fileContent   string
		wantErr       bool
		expectedRoles int
	}{
		{
			name:      "valid file with multiple roles",
			setupFile: true,
			filePath:  "test_roles.yaml",
			fileContent: `roles:
  - name: test-admin
    actions:
      - "*"
  - name: viewer
    actions:
      - "component:view"
      - "project:view"
`,
			wantErr:       false,
			expectedRoles: 2,
		},
		{
			name:          "empty path falls back to embedded roles",
			setupFile:     false,
			filePath:      "",
			wantErr:       false,
			expectedRoles: -1,
		},
		{
			name:      "non-existent file returns error",
			setupFile: false,
			filePath:  "/non/existent/path/roles.yaml",
			wantErr:   true,
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
			roles, err := LoadRolesFromFile(testFilePath)

			if (err != nil) != tt.wantErr {
				t.Errorf("LoadRolesFromFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check expected roles count if specified
			if tt.expectedRoles >= 0 && len(roles) != tt.expectedRoles {
				t.Errorf("LoadRolesFromFile() returned %d roles, want %d", len(roles), tt.expectedRoles)
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
