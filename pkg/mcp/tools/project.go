// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func (t *Toolsets) RegisterListProjects(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_projects"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewProject}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all projects in an namespace. Projects are logical groupings of related " +
			"components that share deployment pipelines. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": stringProperty("Use list_namespaces to discover valid names"),
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

func (t *Toolsets) RegisterCreateProject(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_project"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateProject}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new project in an namespace. Project names must be DNS-compatible " +
			"(lowercase, alphanumeric, hyphens only, max 63 chars).",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name": stringProperty(
				"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"description": stringProperty("Human-readable description"),
			"deployment_pipeline": stringProperty(
				"Name of the DeploymentPipeline to use. Defaults to \"default\" if not specified."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName      string `json:"namespace_name"`
		Name               string `json:"name"`
		Description        string `json:"description"`
		DeploymentPipeline string `json:"deployment_pipeline"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := map[string]string{}
		if args.Description != "" {
			annotations["openchoreo.dev/description"] = args.Description
		}

		projectReq := &gen.CreateProjectJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
		}
		if args.DeploymentPipeline != "" {
			projectReq.Spec = &gen.ProjectSpec{
				DeploymentPipelineRef: &struct {
					Kind *gen.ProjectSpecDeploymentPipelineRefKind `json:"kind,omitempty"`
					Name string                                    `json:"name"`
				}{
					Name: args.DeploymentPipeline,
				},
			}
		}
		result, err := t.ProjectToolset.CreateProject(ctx, args.NamespaceName, projectReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteProject(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_project"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteProject}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete a project. Destructive: cascades to remove all components, workloads, releases, " +
			"and bindings owned by the project. Confirm with the user before calling.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   stringProperty("Use list_projects to discover valid names"),
		}, []string{"namespace_name", "project_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ProjectToolset.DeleteProject(ctx, args.NamespaceName, args.ProjectName)
		return handleToolResult(result, err)
	})
}
