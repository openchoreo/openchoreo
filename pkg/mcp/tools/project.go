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
		Description: "List all projects in an namespace. Projects are logical groupings of related " +
			"components that share deployment pipelines. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": stringProperty("Use get_namespace to discover valid names"),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ProjectToolset.ListProjects(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateProject(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_project",
		Description: "Create a new project in an namespace. Project names must be DNS-compatible " +
			"(lowercase, alphanumeric, hyphens only, max 63 chars).",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name": stringProperty(
				"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"description": stringProperty("Human-readable description"),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
		Description   string `json:"description"`
	}) (*mcp.CallToolResult, any, error) {
		projectReq := &models.CreateProjectRequest{
			Name:        args.Name,
			Description: args.Description,
		}
		result, err := t.ProjectToolset.CreateProject(ctx, args.NamespaceName, projectReq)
		return handleToolResult(result, err)
	})
}
