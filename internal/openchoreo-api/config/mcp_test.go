// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

func TestNewMCPConfig_Defaults(t *testing.T) {
	cfg := MCPDefaults()

	if !cfg.Enabled {
		t.Error("expected Enabled to be true by default")
	}

	expectedToolsets := []string{"namespace", "project", "component", "deployment", "build"}
	if diff := cmp.Diff(expectedToolsets, cfg.Toolsets); diff != "" {
		t.Errorf("default toolsets mismatch (-want +got):\n%s", diff)
	}
}

func TestNewMCPConfig_ValidateToolsets(t *testing.T) {
	tests := []struct {
		name           string
		cfg            MCPConfig
		expectedErrors config.ValidationErrors
	}{
		{
			name: "empty toolsets is valid",
			cfg: MCPConfig{
				Enabled:  true,
				Toolsets: []string{},
			},
			expectedErrors: nil,
		},
		{
			name: "all valid toolsets",
			cfg: MCPConfig{
				Enabled:  true,
				Toolsets: []string{"namespace", "project", "component", "deployment", "build", "pe"},
			},
			expectedErrors: nil,
		},
		{
			name: "single invalid toolset",
			cfg: MCPConfig{
				Enabled:  true,
				Toolsets: []string{"invalid"},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "mcp.toolsets[0]", Message: `unknown toolset "invalid"; valid toolsets: namespace, project, component, deployment, build, pe`},
			},
		},
		{
			name: "mixed valid and invalid toolsets",
			cfg: MCPConfig{
				Enabled:  true,
				Toolsets: []string{"namespace", "unknown", "component"},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "mcp.toolsets[1]", Message: `unknown toolset "unknown"; valid toolsets: namespace, project, component, deployment, build, pe`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.cfg.ValidateMCPConfig(config.NewPath("mcp"))
			if diff := cmp.Diff(tt.expectedErrors, errs); diff != "" {
				t.Errorf("validation errors mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewMCPConfig_ParseToolsets(t *testing.T) {
	cfg := &MCPConfig{
		Toolsets: []string{"namespace", "pe", "build"},
	}

	result := cfg.ParseToolsets()

	expected := map[tools.ToolsetType]bool{
		tools.ToolsetNamespace: true,
		tools.ToolsetPE:        true,
		tools.ToolsetBuild:     true,
	}

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("ParseToolsets mismatch (-want +got):\n%s", diff)
	}

	// Empty toolsets
	empty := (&MCPConfig{Toolsets: []string{}}).ParseToolsets()
	if len(empty) != 0 {
		t.Errorf("expected empty map for empty toolsets, got %v", empty)
	}
}
