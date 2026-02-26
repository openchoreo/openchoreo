// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/openchoreo/openchoreo/internal/config"
)

func TestMCPConfig_Validate(t *testing.T) {
	tests := []struct {
		name           string
		cfg            LegacyMCPConfig
		expectedErrors config.ValidationErrors
	}{
		{
			name: "empty toolsets is valid",
			cfg: LegacyMCPConfig{
				Enabled:  true,
				Toolsets: []string{},
			},
			expectedErrors: nil,
		},
		{
			name: "nil toolsets is valid",
			cfg: LegacyMCPConfig{
				Enabled:  true,
				Toolsets: nil,
			},
			expectedErrors: nil,
		},
		{
			name: "valid toolsets",
			cfg: LegacyMCPConfig{
				Enabled: true,
				Toolsets: []string{
					"namespace",
					"project",
					"component",
					"deployment",
					"infrastructure",
					"schema",
					"resource",
				},
			},
			expectedErrors: nil,
		},
		{
			name: "single valid toolset",
			cfg: LegacyMCPConfig{
				Enabled:  true,
				Toolsets: []string{"namespace"},
			},
			expectedErrors: nil,
		},
		{
			name: "single invalid toolset",
			cfg: LegacyMCPConfig{
				Enabled:  true,
				Toolsets: []string{"invalid"},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "legacy_mcp.toolsets[0]", Message: `unknown toolset "invalid"; valid legacy toolsets: namespace, project, component, deployment, infrastructure, schema, resource`},
			},
		},
		{
			name: "multiple invalid toolsets",
			cfg: LegacyMCPConfig{
				Enabled:  true,
				Toolsets: []string{"foo", "bar"},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "legacy_mcp.toolsets[0]", Message: `unknown toolset "foo"; valid legacy toolsets: namespace, project, component, deployment, infrastructure, schema, resource`},
				{Field: "legacy_mcp.toolsets[1]", Message: `unknown toolset "bar"; valid legacy toolsets: namespace, project, component, deployment, infrastructure, schema, resource`},
			},
		},
		{
			name: "mixed valid and invalid toolsets",
			cfg: LegacyMCPConfig{
				Enabled:  true,
				Toolsets: []string{"namespace", "invalid", "project"},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "legacy_mcp.toolsets[1]", Message: `unknown toolset "invalid"; valid legacy toolsets: namespace, project, component, deployment, infrastructure, schema, resource`},
			},
		},
		{
			name: "disabled with invalid toolsets still validates",
			cfg: LegacyMCPConfig{
				Enabled:  false,
				Toolsets: []string{"invalid"},
			},
			expectedErrors: config.ValidationErrors{
				{Field: "legacy_mcp.toolsets[0]", Message: `unknown toolset "invalid"; valid legacy toolsets: namespace, project, component, deployment, infrastructure, schema, resource`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.cfg.ValidateLegacyMCPConfig(config.NewPath("legacy_mcp"))
			if diff := cmp.Diff(tt.expectedErrors, errs); diff != "" {
				t.Errorf("validation errors mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
