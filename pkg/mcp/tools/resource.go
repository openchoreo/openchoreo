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
func (t *Toolsets) RegisterListResources(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_resources"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewResource}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List resources in a project. Resources reference a ResourceType or " +
			"ClusterResourceType template and represent managed infrastructure (databases, queues, " +
			"caches) consumed by workloads via dependencies.resources[]. Supports pagination via " +
			"limit and cursor.",
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
		result, err := t.ResourceToolset.ListResources(
			ctx, args.NamespaceName, args.ProjectName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetResource(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_resource"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewResource}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the full definition of a resource including its type reference, parameters, " +
			"and the latest ResourceRelease pointer. Use list_resources to discover valid names.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Resource name. Use list_resources to discover valid names"),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ResourceToolset.GetResource(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateResource(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_resource"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateResource}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new resource in a project. The resource references a ResourceType " +
			"(namespace-scoped) or ClusterResourceType (cluster-scoped) template by name. Parameters " +
			"must conform to the template's parameters schema; use get_resource_type_schema with the " +
			"matching scope to inspect it.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"name": stringProperty(
				"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"type_name": stringProperty(
				"Name of the (Cluster)ResourceType to reference. Use list_resource_types to discover names."),
			"type_kind": stringProperty(
				"Optional: \"ResourceType\" (default, namespace-scoped) or \"ClusterResourceType\" (cluster-scoped)."),
			"display_name": stringProperty("Human-readable display name"),
			"description":  stringProperty("Human-readable description"),
			"parameters": map[string]any{
				"type":        "object",
				"description": "Optional: parameter values for the referenced (Cluster)ResourceType schema.",
			},
		}, []string{"namespace_name", "project_name", "name", "type_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		ProjectName   string                 `json:"project_name"`
		Name          string                 `json:"name"`
		TypeName      string                 `json:"type_name"`
		TypeKind      string                 `json:"type_kind"`
		DisplayName   string                 `json:"display_name"`
		Description   string                 `json:"description"`
		Parameters    map[string]interface{} `json:"parameters"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := annotationsFromDisplayDesc(args.DisplayName, args.Description)
		typeRef := gen.ResourceTypeRef{Name: args.TypeName}
		if args.TypeKind != "" {
			kind := gen.ResourceTypeRefKind(args.TypeKind)
			typeRef.Kind = &kind
		}
		body := &gen.CreateResourceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: args.Name, Annotations: &annotations},
			Spec: &gen.ResourceInstanceSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: args.ProjectName},
				Type: typeRef,
			},
		}
		if args.Parameters != nil {
			body.Spec.Parameters = &args.Parameters
		}
		result, err := t.ResourceToolset.CreateResource(ctx, args.NamespaceName, args.ProjectName, body)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterUpdateResource(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_resource"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateResource}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Update an existing resource (partial update). Only provided fields are applied; " +
			"omitted fields remain unchanged. spec.owner and spec.type are immutable and cannot be " +
			"changed after creation.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Resource name to update. Use list_resources to discover valid names"),
			"display_name":   stringProperty("Optional: update the human-readable display name"),
			"description":    stringProperty("Optional: update the human-readable description"),
			"parameters": map[string]any{
				"type":        "object",
				"description": "Optional: replace parameter values for the referenced (Cluster)ResourceType schema.",
			},
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		Name          string                 `json:"name"`
		DisplayName   string                 `json:"display_name"`
		Description   string                 `json:"description"`
		Parameters    map[string]interface{} `json:"parameters"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := annotationsFromDisplayDesc(args.DisplayName, args.Description)
		body := &gen.UpdateResourceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: args.Name, Annotations: &annotations},
		}
		if args.Parameters != nil {
			body.Spec = &gen.ResourceInstanceSpec{Parameters: &args.Parameters}
		}
		result, err := t.ResourceToolset.UpdateResource(ctx, args.NamespaceName, body)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteResource(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_resource"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteResource}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete a resource. The owning project's finalizer cleans up the resource's " +
			"ResourceReleases once any ResourceReleaseBindings are removed.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Resource name to delete. Use list_resources to discover valid names"),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ResourceToolset.DeleteResource(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}

// annotationsFromDisplayDesc builds an annotations map with display name and description entries when set.
// Returns an empty map (not nil) so JSON serialization of the request body is consistent.
func annotationsFromDisplayDesc(displayName, description string) map[string]string {
	a := map[string]string{}
	if displayName != "" {
		a["openchoreo.dev/display-name"] = displayName
	}
	if description != "" {
		a["openchoreo.dev/description"] = description
	}
	return a
}
