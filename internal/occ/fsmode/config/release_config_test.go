// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadReleaseConfig(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantErr   string
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
			checkFunc: func(t *testing.T, cfg *ReleaseConfig) {
				require.NotNil(t, cfg.ComponentReleaseDefaults)
				assert.Equal(t, "./releases", cfg.ComponentReleaseDefaults.DefaultOutputDir)
				assert.Len(t, cfg.ComponentReleaseDefaults.Projects, 2)
				assert.Equal(t, "./projects/demo/releases", cfg.ComponentReleaseDefaults.Projects["demo-project"].DefaultOutputDir)
			},
		},
		{
			name: "minimal valid config",
			content: `apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseConfig
`,
			checkFunc: func(t *testing.T, cfg *ReleaseConfig) {
				// ComponentReleaseDefaults can be nil for minimal config
			},
		},
		{
			name: "missing apiVersion",
			content: `kind: ReleaseConfig
defaultOutputDir: ./releases
`,
			wantErr: "apiVersion is required",
		},
		{
			name: "missing kind",
			content: `apiVersion: openchoreo.dev/v1alpha1
defaultOutputDir: ./releases
`,
			wantErr: "kind is required",
		},
		{
			name: "wrong apiVersion",
			content: `apiVersion: v1
kind: ReleaseConfig
`,
			wantErr: "unsupported apiVersion",
		},
		{
			name: "wrong kind",
			content: `apiVersion: openchoreo.dev/v1alpha1
kind: WrongKind
`,
			wantErr: "unsupported kind",
		},
		{
			name: "invalid YAML",
			content: `apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseConfig
invalid: [unclosed
`,
			wantErr: "failed to parse config file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte(tt.content), 0600))

			cfg, err := LoadReleaseConfig(configPath)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.checkFunc != nil {
				tt.checkFunc(t, cfg)
			}
		})
	}
}

func TestLoadReleaseConfig_FileNotFound(t *testing.T) {
	_, err := LoadReleaseConfig("/nonexistent/path/config.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config file")
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
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetReleaseOutputDir_NilConfig(t *testing.T) {
	var cfg *ReleaseConfig
	got := cfg.GetReleaseOutputDir("project", "component")
	assert.Empty(t, got)
}

func TestGetReleaseOutputDir_NilComponentReleaseDefaults(t *testing.T) {
	cfg := &ReleaseConfig{
		APIVersion:               "openchoreo.dev/v1alpha1",
		Kind:                     "ReleaseConfig",
		ComponentReleaseDefaults: nil,
	}
	got := cfg.GetReleaseOutputDir("project", "component")
	assert.Empty(t, got)
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  ReleaseConfig
		wantErr string
	}{
		{
			name: "valid config",
			config: ReleaseConfig{
				APIVersion: "openchoreo.dev/v1alpha1",
				Kind:       "ReleaseConfig",
			},
		},
		{
			name:    "missing apiVersion",
			config:  ReleaseConfig{Kind: "ReleaseConfig"},
			wantErr: "apiVersion is required",
		},
		{
			name:    "missing kind",
			config:  ReleaseConfig{APIVersion: "openchoreo.dev/v1alpha1"},
			wantErr: "kind is required",
		},
		{
			name:    "wrong apiVersion",
			config:  ReleaseConfig{APIVersion: "v1", Kind: "ReleaseConfig"},
			wantErr: "unsupported apiVersion",
		},
		{
			name:    "wrong kind",
			config:  ReleaseConfig{APIVersion: "openchoreo.dev/v1alpha1", Kind: "ConfigMap"},
			wantErr: "unsupported kind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
