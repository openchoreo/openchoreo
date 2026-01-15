// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// MCPConfig defines Model Context Protocol server settings.
type MCPConfig struct {
	// Enabled enables the MCP server endpoint.
	Enabled bool `koanf:"enabled"`
	// Toolsets is the list of enabled MCP toolsets.
	Toolsets []string `koanf:"toolsets"`
	// OAuth defines OAuth settings for MCP protected resource metadata.
	OAuth MCPOAuthConfig `koanf:"oauth"`
}

// MCPOAuthConfig defines OAuth settings for MCP.
type MCPOAuthConfig struct {
	// ResourceURL is the base URL of the MCP resource (typically the API server URL).
	ResourceURL string `koanf:"resource_url"`
	// AuthServerURL is the base URL of the authorization server.
	AuthServerURL string `koanf:"auth_server_url"`
}

// MCPDefaults returns the default MCP configuration.
func MCPDefaults() MCPConfig {
	return MCPConfig{
		Enabled: true,
		Toolsets: []string{
			string(tools.ToolsetOrganization),
			string(tools.ToolsetProject),
			string(tools.ToolsetComponent),
			string(tools.ToolsetBuild),
			string(tools.ToolsetDeployment),
			string(tools.ToolsetInfrastructure),
			string(tools.ToolsetSchema),
			string(tools.ToolsetResource),
		},
		OAuth: MCPOAuthConfig{
			ResourceURL:   "http://api.openchoreo.localhost",
			AuthServerURL: "http://sts.openchoreo.localhost",
		},
	}
}

// Validate validates the MCP configuration.
func (c *MCPConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if !c.Enabled {
		return errs // skip validation if disabled
	}

	if c.OAuth.ResourceURL == "" {
		errs = append(errs, config.Required(path.Child("oauth").Child("resource_url")))
	}

	if c.OAuth.AuthServerURL == "" {
		errs = append(errs, config.Required(path.Child("oauth").Child("auth_server_url")))
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
