// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

func (t *Toolsets) RegisterListProjects(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_projects",
		Description: "List all projects in an organization. Projects are logical groupings of related " +
			"components that share deployment pipelines.",
		InputSchema: createSchema(map[string]any{
			"org_name": stringProperty("Use get_organization to discover valid names"),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ProjectToolset.ListProjects(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetProject(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_project",
		Description: "Get detailed information about a specific project including deployment pipeline " +
			"configuration and component summary.",
		InputSchema: createSchema(map[string]any{
			"org_name":     defaultStringProperty(),
			"project_name": stringProperty("Use list_projects to discover valid names"),
		}, []string{"org_name", "project_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName     string `json:"org_name"`
		ProjectName string `json:"project_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ProjectToolset.GetProject(ctx, args.OrgName, args.ProjectName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateProject(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_project",
		Description: "Create a new project in an organization. Project names must be DNS-compatible " +
			"(lowercase, alphanumeric, hyphens only, max 63 chars).",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
			"name": stringProperty(
				"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"description": stringProperty("Human-readable description"),
		}, []string{"org_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName     string `json:"org_name"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}) (*mcp.CallToolResult, any, error) {
		projectReq := &models.CreateProjectRequest{
			Name:        args.Name,
			Description: args.Description,
		}
		result, err := t.ProjectToolset.CreateProject(ctx, args.OrgName, projectReq)
		return handleToolResult(result, err)
	})
}
