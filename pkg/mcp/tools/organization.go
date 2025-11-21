// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (t *Toolsets) RegisterGetOrganization(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_organization",
		Description: "Get information about a specific organization by name. " +
			"Organizations are the top-level tenant boundary containing projects, " +
			"environments, and infrastructure.",
		InputSchema: createSchema(map[string]any{
			"name": defaultStringProperty(),
		}, []string{"name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.OrganizationToolset.GetOrganization(ctx, args.Name)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListOrganizations(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_organizations",
		Description: "List all accessible organizations. Organizations are the top-level " +
			"tenant boundary containing projects, environments, and infrastructure.",
		InputSchema: createSchema(map[string]any{}, []string{}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.OrganizationToolset.ListOrganizations(ctx)
		return handleToolResult(result, err)
	})
}
