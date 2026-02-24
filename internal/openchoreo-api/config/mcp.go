// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/pkg/mcp/legacytools"
)

// LegacyMCPConfig defines Model Context Protocol server settings.
type LegacyMCPConfig struct {
	// Enabled enables the MCP server endpoint.
	Enabled bool `koanf:"enabled"`
	// Toolsets is the list of enabled MCP toolsets.
	Toolsets []string `koanf:"toolsets"`
}

// LegacyMCPDefaults returns the default MCP configuration.
func LegacyMCPDefaults() LegacyMCPConfig {
	return LegacyMCPConfig{
		Enabled: true,
		Toolsets: []string{
			string(legacytools.ToolsetNamespace),
			string(legacytools.ToolsetProject),
			string(legacytools.ToolsetComponent),
			string(legacytools.ToolsetBuild),
			string(legacytools.ToolsetDeployment),
			string(legacytools.ToolsetInfrastructure),
			string(legacytools.ToolsetSchema),
			string(legacytools.ToolsetResource),
		},
	}
}

// validLegacyToolsets is the set of valid MCP toolset names.
var validLegacyToolsets = map[string]bool{
	string(legacytools.ToolsetNamespace):      true,
	string(legacytools.ToolsetProject):        true,
	string(legacytools.ToolsetComponent):      true,
	string(legacytools.ToolsetBuild):          true,
	string(legacytools.ToolsetDeployment):     true,
	string(legacytools.ToolsetInfrastructure): true,
	string(legacytools.ToolsetSchema):         true,
	string(legacytools.ToolsetResource):       true,
}

// ValidateLegacyMCPConfig validates the MCP configuration.
func (c *LegacyMCPConfig) ValidateLegacyMCPConfig(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	for i, ts := range c.Toolsets {
		if !validLegacyToolsets[ts] {
			errs = append(errs, config.Invalid(path.Child("toolsets").Index(i),
				fmt.Sprintf("unknown toolset %q; valid legacy toolsets: namespace, project, component, build, deployment, infrastructure, schema, resource", ts)))
		}
	}

	return errs
}

// ParseLegacyToolsets converts the toolset strings to a map of ToolsetType for lookup.
func (c *LegacyMCPConfig) ParseLegacyToolsets() map[legacytools.ToolsetType]bool {
	result := make(map[legacytools.ToolsetType]bool, len(c.Toolsets))
	for _, ts := range c.Toolsets {
		result[legacytools.ToolsetType(ts)] = true
	}
	return result
}
