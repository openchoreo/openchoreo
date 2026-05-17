// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

//nolint:dupl // paginated list handlers share similar structure
func (t *Toolsets) RegisterListResourceReleaseBindings(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_resource_release_bindings"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewResourceReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List ResourceReleaseBindings for a resource. Each binding pins a ResourceRelease " +
			"to an environment and carries per-env configuration overrides. Supports pagination via " +
			"limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
			"resource_name":  defaultStringProperty(),
		}), []string{"namespace_name", "resource_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ResourceName  string `json:"resource_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.ListResourceReleaseBindings(
			ctx, args.NamespaceName, args.ResourceName,
			ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetResourceReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_resource_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewResourceReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the full definition of a ResourceReleaseBinding including the current release pin, " +
			"per-env configuration overrides, retain policy, conditions, and resolved outputs.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name": stringProperty(
				"Binding name. Use list_resource_release_bindings to discover valid names."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.GetResourceReleaseBinding(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateResourceReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_resource_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateResourceReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a ResourceReleaseBinding that pins a ResourceRelease to a specific environment. " +
			"spec.owner and spec.environment are immutable after creation. The release pin " +
			"(spec.resourceRelease) is advanced manually via update_resource_release_binding.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Binding name."),
			"project_name":   stringProperty("Name of the project that owns the resource."),
			"resource_name":  stringProperty("Name of the parent Resource."),
			"environment":    stringProperty("Target environment name."),
			"resource_release": stringProperty(
				"Optional: name of the ResourceRelease to pin. Leave unset for a pending binding."),
			"retain_policy": stringProperty(
				"Optional: per-env retention override. \"Delete\" or \"Retain\". " +
					"Falls back to ResourceType.spec.retainPolicy when unset."),
			"resource_type_environment_configs": map[string]any{
				"type": "object",
				"description": "Optional: per-env values for the referenced ResourceType's " +
					"environmentConfigs schema. Use get_resource_type_schema for the matching scope.",
			},
		}, []string{"namespace_name", "name", "project_name", "resource_name", "environment"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName                  string                 `json:"namespace_name"`
		Name                           string                 `json:"name"`
		ProjectName                    string                 `json:"project_name"`
		ResourceName                   string                 `json:"resource_name"`
		Environment                    string                 `json:"environment"`
		ResourceRelease                string                 `json:"resource_release"`
		RetainPolicy                   string                 `json:"retain_policy"`
		ResourceTypeEnvironmentConfigs map[string]interface{} `json:"resource_type_environment_configs"`
	}) (*mcp.CallToolResult, any, error) {
		spec := gen.ResourceReleaseBindingSpec{
			Environment: args.Environment,
			Owner: struct {
				ProjectName  string `json:"projectName"`
				ResourceName string `json:"resourceName"`
			}{
				ProjectName:  args.ProjectName,
				ResourceName: args.ResourceName,
			},
		}
		if args.ResourceRelease != "" {
			spec.ResourceRelease = &args.ResourceRelease
		}
		retainPolicy, err := parseResourceRetainPolicy(args.RetainPolicy)
		if err != nil {
			return nil, nil, err
		}
		if retainPolicy != nil {
			spec.RetainPolicy = retainPolicy
		}
		if args.ResourceTypeEnvironmentConfigs != nil {
			spec.ResourceTypeEnvironmentConfigs = &args.ResourceTypeEnvironmentConfigs
		}
		body := &gen.CreateResourceReleaseBindingJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: args.Name},
			Spec:     &spec,
		}
		result, err := t.DeploymentToolset.CreateResourceReleaseBinding(ctx, args.NamespaceName, body)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterUpdateResourceReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_resource_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateResourceReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Update an existing ResourceReleaseBinding (partial update). spec.owner and " +
			"spec.environment cannot be changed. Use this to advance the release pin (promote a new " +
			"ResourceRelease into an environment), change the retain policy, or update environment " +
			"configuration overrides.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Binding name. Use list_resource_release_bindings to discover valid names."),
			"resource_release": stringProperty(
				"Optional: name of the ResourceRelease to pin. Use list_resource_releases to discover names."),
			"retain_policy": stringProperty(
				"Optional: per-env retention override. \"Delete\" or \"Retain\"."),
			"resource_type_environment_configs": map[string]any{
				"type":        "object",
				"description": "Optional: replace per-env values for the referenced ResourceType's environmentConfigs.",
			},
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName                  string                 `json:"namespace_name"`
		Name                           string                 `json:"name"`
		ResourceRelease                string                 `json:"resource_release"`
		RetainPolicy                   string                 `json:"retain_policy"`
		ResourceTypeEnvironmentConfigs map[string]interface{} `json:"resource_type_environment_configs"`
	}) (*mcp.CallToolResult, any, error) {
		spec := &gen.ResourceReleaseBindingSpec{}
		if args.ResourceRelease != "" {
			spec.ResourceRelease = &args.ResourceRelease
		}
		retainPolicy, err := parseResourceRetainPolicy(args.RetainPolicy)
		if err != nil {
			return nil, nil, err
		}
		if retainPolicy != nil {
			spec.RetainPolicy = retainPolicy
		}
		if args.ResourceTypeEnvironmentConfigs != nil {
			spec.ResourceTypeEnvironmentConfigs = &args.ResourceTypeEnvironmentConfigs
		}
		body := &gen.UpdateResourceReleaseBindingJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: args.Name},
			Spec:     spec,
		}
		result, err := t.DeploymentToolset.UpdateResourceReleaseBinding(ctx, args.NamespaceName, body)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteResourceReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_resource_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteResourceReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete a ResourceReleaseBinding. When retainPolicy=Delete (default), the underlying " +
			"RenderedRelease is cascaded for cleanup; when retainPolicy=Retain, the finalizer blocks " +
			"until manually removed.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Binding name to delete. Use list_resource_release_bindings to discover names."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.DeleteResourceReleaseBinding(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}

func parseResourceRetainPolicy(raw string) (*gen.ResourceReleaseBindingSpecRetainPolicy, error) {
	if raw == "" {
		return nil, nil
	}
	switch raw {
	case string(gen.ResourceReleaseBindingSpecRetainPolicyDelete),
		string(gen.ResourceReleaseBindingSpecRetainPolicyRetain):
		policy := gen.ResourceReleaseBindingSpecRetainPolicy(raw)
		return &policy, nil
	default:
		return nil, fmt.Errorf("retain_policy must be one of: Delete, Retain")
	}
}
