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

func (t *Toolsets) RegisterListSecretReferences(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_secret_references",
		Description: "List all secret references for an organization. Secret references are " +
			"credentials and sensitive configuration that can be used by components.",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.OrganizationToolset.ListSecretReferences(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}
