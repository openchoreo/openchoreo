// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	apiaudit "github.com/openchoreo/openchoreo/internal/openchoreo-api/audit"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// TestMCPBindings_MatchRegisteredTools guards against a typo'd or renamed
// MCPToolName in operationDefs leaving that tool silently unaudited forever —
// the MCP equivalent of BuildPatternMap's operationId-vs-spec check on REST.
//
// This registers every toolset, not just whatever subset a given deployment
// enables (e.g. "pe" — which owns the environment tools — is absent from
// config.MCPDefaults' toolset list), so a binding whose toolset is merely
// disabled in some deployment's config doesn't look like a bug here. That
// deployment-specific check belongs in NewHTTPServer at runtime, not this
// test: whether a bound tool is actually reachable depends on which toolsets
// an operator enabled, which this test deliberately ignores by enabling all
// of them.
func TestMCPBindings_MatchRegisteredTools(t *testing.T) {
	handler := NewMCPHandler(&handlerservices.Services{})
	toolsets := &tools.Toolsets{
		NamespaceToolset:  handler,
		ProjectToolset:    handler,
		ComponentToolset:  handler,
		DeploymentToolset: handler,
		BuildToolset:      handler,
		PEToolset:         handler,
		ResourceToolset:   handler,
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	perms, _ := toolsets.Register(server)

	for tool := range apiaudit.MCPBindings() {
		if _, ok := perms[tool]; !ok {
			t.Errorf("MCPBindings() has an entry for tool %q, but no RegisterFunc registers a tool "+
				"by that name (typo or renamed tool in operationDefs' MCPToolName?)", tool)
		}
	}
}
