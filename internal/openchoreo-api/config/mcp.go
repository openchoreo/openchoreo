// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/pkg/mcp/legacytools"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
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
				fmt.Sprintf("unknown toolset %q; valid legacy toolsets: namespace, project, component, deployment, infrastructure, schema, resource", ts)))
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

// MCPConfig defines Model Context Protocol server settings.
type MCPConfig struct {
	// Enabled enables the MCP server endpoint.
	Enabled bool `koanf:"enabled"`
	// Toolsets is the list of enabled MCP toolsets.
	Toolsets []string `koanf:"toolsets"`
}

// MCPDefaults returns the default MCP configuration.
func MCPDefaults() MCPConfig {
	return MCPConfig{
		Enabled: true,
		Toolsets: []string{
			string(tools.ToolsetNamespace),
			string(tools.ToolsetProject),
			string(tools.ToolsetComponent),
			string(tools.ToolsetInfrastructure),
		},
	}
}

// validToolsets is the set of valid MCP toolset names.
var validToolsets = map[string]bool{
	string(tools.ToolsetNamespace):      true,
	string(tools.ToolsetProject):        true,
	string(tools.ToolsetComponent):      true,
	string(tools.ToolsetInfrastructure): true,
	string(tools.ToolsetPE):             true,
}

// ValidateMCPConfig validates the MCP configuration.
func (c *MCPConfig) ValidateMCPConfig(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	for i, ts := range c.Toolsets {
		if !validToolsets[ts] {
			errs = append(errs, config.Invalid(path.Child("toolsets").Index(i),
				fmt.Sprintf("unknown toolset %q; valid toolsets: namespace, project, component, infrastructure, pe", ts)))
		}
	}

	return errs
}

// ParseToolsets converts the toolset strings to a map of ToolsetType for lookup.
func (c *MCPConfig) ParseToolsets() map[tools.ToolsetType]bool {
	result := make(map[tools.ToolsetType]bool, len(c.Toolsets))
	for _, ts := range c.Toolsets {
		result[tools.ToolsetType(ts)] = true
	}
	return result
}
