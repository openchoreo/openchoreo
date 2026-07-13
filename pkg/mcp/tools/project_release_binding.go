// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

//nolint:dupl // paginated list handlers share similar structure
func (t *Toolsets) RegisterListProjectReleaseBindings(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_project_release_bindings"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewProjectReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List ProjectReleaseBindings for a project. Each binding pins a ProjectRelease " +
			"to an environment, owns the cell namespace for that (project, environment), and carries " +
			"per-env configuration overrides. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
		}), []string{"namespace_name", "project_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.ListProjectReleaseBindings(
			ctx, args.NamespaceName, args.ProjectName,
			ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetProjectReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_project_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewProjectReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the full definition of a ProjectReleaseBinding including the current release pin, " +
			"per-env configuration overrides, conditions, and the owned data-plane namespace.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name": stringProperty(
				"Binding name. Use list_project_release_bindings to discover valid names."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.GetProjectReleaseBinding(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateProjectReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_project_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateProjectReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a ProjectReleaseBinding that pins a ProjectRelease to a specific environment. " +
			"spec.owner and spec.environment are immutable after creation. The release pin " +
			"(spec.projectRelease) is advanced manually via update_project_release_binding.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Binding name."),
			"project_name":   stringProperty("Name of the project that owns the binding."),
			"environment":    stringProperty("Target environment name."),
			"project_release": stringProperty(
				"Optional: name of the ProjectRelease to pin. Leave unset for a pending binding."),
			"environment_configs": map[string]any{
				"type": "object",
				"description": "Optional: per-env values for the referenced (Cluster)ProjectType's " +
					"environmentConfigs schema. Use get_project_type_schema for the matching scope.",
			},
		}, []string{"namespace_name", "name", "project_name", "environment"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName      string                 `json:"namespace_name"`
		Name               string                 `json:"name"`
		ProjectName        string                 `json:"project_name"`
		Environment        string                 `json:"environment"`
		ProjectRelease     string                 `json:"project_release"`
		EnvironmentConfigs map[string]interface{} `json:"environment_configs"`
	}) (*mcp.CallToolResult, any, error) {
		spec := gen.ProjectReleaseBindingSpec{
			Environment: args.Environment,
			Owner: struct {
				ProjectName string `json:"projectName"`
			}{
				ProjectName: args.ProjectName,
			},
		}
		if args.ProjectRelease != "" {
			spec.ProjectRelease = &args.ProjectRelease
		}
		if args.EnvironmentConfigs != nil {
			spec.EnvironmentConfigs = &args.EnvironmentConfigs
		}
		body := &gen.CreateProjectReleaseBindingJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: args.Name},
			Spec:     &spec,
		}
		result, err := t.DeploymentToolset.CreateProjectReleaseBinding(ctx, args.NamespaceName, body)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterUpdateProjectReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_project_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateProjectReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Update an existing ProjectReleaseBinding (partial update). spec.owner and " +
			"spec.environment cannot be changed. Use this to advance the release pin (promote a new " +
			"ProjectRelease into an environment) or update environment configuration overrides.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Binding name. Use list_project_release_bindings to discover valid names."),
			"project_release": stringProperty(
				"Optional: name of the ProjectRelease to pin. Use list_project_releases to discover names."),
			"environment_configs": map[string]any{
				"type":        "object",
				"description": "Optional: replace per-env values for the referenced (Cluster)ProjectType's environmentConfigs.",
			},
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName      string                 `json:"namespace_name"`
		Name               string                 `json:"name"`
		ProjectRelease     string                 `json:"project_release"`
		EnvironmentConfigs map[string]interface{} `json:"environment_configs"`
	}) (*mcp.CallToolResult, any, error) {
		spec := &gen.ProjectReleaseBindingSpec{}
		if args.ProjectRelease != "" {
			spec.ProjectRelease = &args.ProjectRelease
		}
		if args.EnvironmentConfigs != nil {
			spec.EnvironmentConfigs = &args.EnvironmentConfigs
		}
		body := &gen.UpdateProjectReleaseBindingJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: args.Name},
			Spec:     spec,
		}
		result, err := t.DeploymentToolset.UpdateProjectReleaseBinding(ctx, args.NamespaceName, body)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteProjectReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_project_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteProjectReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete a ProjectReleaseBinding. Cascades cleanup of the cell namespace and the " +
			"rendered resources owned by this (project, environment) binding.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Binding name to delete. Use list_project_release_bindings to discover names."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.DeleteProjectReleaseBinding(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}
