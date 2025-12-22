// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReleaseConfig(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantErr   bool
		errMsg    string
		checkFunc func(*testing.T, *ReleaseConfig)
	}{
		{
			name: "valid config with all fields",
			content: `apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseConfig
componentReleaseDefaults:
  defaultOutputDir: ./releases
  projects:
    demo-project:
      defaultOutputDir: ./projects/demo/releases
      components:
        greeter: ./projects/demo/components/greeter/releases
    ecommerce-demo:
      defaultOutputDir: ./projects/ecommerce/releases
`,
			wantErr: false,
			checkFunc: func(t *testing.T, cfg *ReleaseConfig) {
				if cfg.ComponentReleaseDefaults == nil {
					t.Fatal("expected componentReleaseDefaults to be non-nil")
				}
				if cfg.ComponentReleaseDefaults.DefaultOutputDir != "./releases" {
					t.Errorf("expected defaultOutputDir './releases', got '%s'", cfg.ComponentReleaseDefaults.DefaultOutputDir)
				}
				if len(cfg.ComponentReleaseDefaults.Projects) != 2 {
					t.Errorf("expected 2 projects, got %d", len(cfg.ComponentReleaseDefaults.Projects))
				}
				if cfg.ComponentReleaseDefaults.Projects["demo-project"].DefaultOutputDir != "./projects/demo/releases" {
					t.Errorf("unexpected project defaultOutputDir")
				}
			},
		},
		{
			name: "minimal valid config",
			content: `apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseConfig
`,
			wantErr: false,
			checkFunc: func(t *testing.T, cfg *ReleaseConfig) {
				// ComponentReleaseDefaults can be nil for minimal config
				// This is valid - it means no custom output directories configured
			},
		},
		{
			name: "missing apiVersion",
			content: `kind: ReleaseConfig
defaultOutputDir: ./releases
`,
			wantErr: true,
			errMsg:  "apiVersion is required",
		},
		{
			name: "missing kind",
			content: `apiVersion: openchoreo.dev/v1alpha1
defaultOutputDir: ./releases
`,
			wantErr: true,
			errMsg:  "kind is required",
		},
		{
			name: "wrong apiVersion",
			content: `apiVersion: v1
kind: ReleaseConfig
`,
			wantErr: true,
			errMsg:  "unsupported apiVersion",
		},
		{
			name: "wrong kind",
			content: `apiVersion: openchoreo.dev/v1alpha1
kind: WrongKind
`,
			wantErr: true,
			errMsg:  "unsupported kind",
		},
		{
			name: "invalid YAML",
			content: `apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseConfig
invalid: [unclosed
`,
			wantErr: true,
			errMsg:  "failed to parse config file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			if err := os.WriteFile(configPath, []byte(tt.content), 0600); err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			// Load config
			cfg, err := LoadReleaseConfig(configPath)

			// Check error
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing '%s', got nil", tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Run custom check function if provided
			if tt.checkFunc != nil {
				tt.checkFunc(t, cfg)
			}
		})
	}
}

func TestLoadReleaseConfig_FileNotFound(t *testing.T) {
	_, err := LoadReleaseConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
	if !contains(err.Error(), "failed to read config file") {
		t.Errorf("expected 'failed to read config file' error, got: %v", err)
	}
}

func TestGetReleaseOutputDir(t *testing.T) {
	cfg := &ReleaseConfig{
		APIVersion: "openchoreo.dev/v1alpha1",
		Kind:       "ReleaseConfig",
		ComponentReleaseDefaults: &ComponentReleaseDefaults{
			DefaultOutputDir: "./global-releases",
			Projects: map[string]ProjectReleaseConfig{
				"demo-project": {
					DefaultOutputDir: "./projects/demo/releases",
					Components: map[string]string{
						"greeter": "./projects/demo/components/greeter/releases",
						"api":     "./projects/demo/components/api/releases",
					},
				},
				"ecommerce-demo": {
					DefaultOutputDir: "./projects/ecommerce/releases",
				},
				"minimal-project": {},
			},
		},
	}

	tests := []struct {
		name          string
		projectName   string
		componentName string
		want          string
	}{
		{
			name:          "component-specific override",
			projectName:   "demo-project",
			componentName: "greeter",
			want:          "./projects/demo/components/greeter/releases",
		},
		{
			name:          "project default (no component override)",
			projectName:   "demo-project",
			componentName: "other-component",
			want:          "./projects/demo/releases",
		},
		{
			name:          "global default (project has no default)",
			projectName:   "minimal-project",
			componentName: "some-component",
			want:          "./global-releases",
		},
		{
			name:          "global default (project not found)",
			projectName:   "nonexistent-project",
			componentName: "some-component",
			want:          "./global-releases",
		},
		{
			name:          "ecommerce project default",
			projectName:   "ecommerce-demo",
			componentName: "order-api",
			want:          "./projects/ecommerce/releases",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfg.GetReleaseOutputDir(tt.projectName, tt.componentName)
			if got != tt.want {
				t.Errorf("GetReleaseOutputDir(%q, %q) = %q, want %q",
					tt.projectName, tt.componentName, got, tt.want)
			}
		})
	}
}

func TestGetReleaseOutputDir_NilConfig(t *testing.T) {
	var cfg *ReleaseConfig = nil
	got := cfg.GetReleaseOutputDir("project", "component")
	if got != "" {
		t.Errorf("expected empty string for nil config, got %q", got)
	}
}

func TestGetReleaseOutputDir_NilComponentReleaseDefaults(t *testing.T) {
	cfg := &ReleaseConfig{
		APIVersion:               "openchoreo.dev/v1alpha1",
		Kind:                     "ReleaseConfig",
		ComponentReleaseDefaults: nil,
	}
	got := cfg.GetReleaseOutputDir("project", "component")
	if got != "" {
		t.Errorf("expected empty string when ComponentReleaseDefaults is nil, got %q", got)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  ReleaseConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: ReleaseConfig{
				APIVersion: "openchoreo.dev/v1alpha1",
				Kind:       "ReleaseConfig",
			},
			wantErr: false,
		},
		{
			name: "missing apiVersion",
			config: ReleaseConfig{
				Kind: "ReleaseConfig",
			},
			wantErr: true,
			errMsg:  "apiVersion is required",
		},
		{
			name: "missing kind",
			config: ReleaseConfig{
				APIVersion: "openchoreo.dev/v1alpha1",
			},
			wantErr: true,
			errMsg:  "kind is required",
		},
		{
			name: "wrong apiVersion",
			config: ReleaseConfig{
				APIVersion: "v1",
				Kind:       "ReleaseConfig",
			},
			wantErr: true,
			errMsg:  "unsupported apiVersion",
		},
		{
			name: "wrong kind",
			config: ReleaseConfig{
				APIVersion: "openchoreo.dev/v1alpha1",
				Kind:       "ConfigMap",
			},
			wantErr: true,
			errMsg:  "unsupported kind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing '%s', got nil", tt.errMsg)
				} else if !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
