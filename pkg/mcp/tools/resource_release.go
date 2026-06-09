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
func (t *Toolsets) RegisterPEListResourceReleases(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_resource_releases"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewResourceRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all releases for a resource. ResourceReleases are immutable snapshots of " +
			"Resource.spec plus the referenced (Cluster)ResourceType.spec at the time they were cut. " +
			"Created automatically by the Resource controller; rarely created by hand. Supports " +
			"pagination via limit and cursor.",
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
		result, err := t.PEToolset.ListResourceReleases(
			ctx, args.NamespaceName, args.ResourceName,
			ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPECreateResourceRelease(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_resource_release"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateResourceRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a ResourceRelease (immutable snapshot of Resource.spec + the referenced " +
			"(Cluster)ResourceType.spec). Normally created by the Resource controller; expose this for " +
			"GitOps-style manual snapshots. The type_spec argument must contain the full ResourceType " +
			"snapshot to embed in the release.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Release name. Convention: {resource}-{hash}."),
			"project_name":   stringProperty("Name of the project that owns the resource."),
			"resource_name":  stringProperty("Name of the parent Resource."),
			"type_name":      stringProperty("Name of the (Cluster)ResourceType the snapshot was taken from."),
			"type_kind": stringProperty(
				"Optional: \"ResourceType\" (default, namespace-scoped) or \"ClusterResourceType\" (cluster-scoped)."),
			"type_spec": map[string]any{
				"type":        "object",
				"description": "Full ResourceTypeSpec snapshot to embed in the release.",
			},
			"parameters": map[string]any{
				"type":        "object",
				"description": "Optional: parameter values captured from the source Resource at release time.",
			},
		}, []string{"namespace_name", "name", "project_name", "resource_name", "type_name", "type_spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		Name          string                 `json:"name"`
		ProjectName   string                 `json:"project_name"`
		ResourceName  string                 `json:"resource_name"`
		TypeName      string                 `json:"type_name"`
		TypeKind      string                 `json:"type_kind"`
		TypeSpec      map[string]interface{} `json:"type_spec"`
		Parameters    map[string]interface{} `json:"parameters"`
	}) (*mcp.CallToolResult, any, error) {
		typeSpec, err := buildSpec[gen.ResourceTypeSpec](args.TypeSpec)
		if err != nil {
			return nil, nil, err
		}
		kind := gen.ResourceReleaseSpecResourceTypeKind(args.TypeKind)
		if args.TypeKind == "" {
			kind = gen.ResourceReleaseSpecResourceTypeKindResourceType
		}
		spec := gen.ResourceReleaseSpec{
			Owner: struct {
				ProjectName  string `json:"projectName"`
				ResourceName string `json:"resourceName"`
			}{
				ProjectName:  args.ProjectName,
				ResourceName: args.ResourceName,
			},
			ResourceType: struct {
				Kind gen.ResourceReleaseSpecResourceTypeKind `json:"kind"`
				Name string                                  `json:"name"`
				Spec gen.ResourceTypeSpec                    `json:"spec"`
			}{
				Kind: kind,
				Name: args.TypeName,
				Spec: *typeSpec,
			},
		}
		if args.Parameters != nil {
			spec.Parameters = &args.Parameters
		}
		body := &gen.CreateResourceReleaseJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: args.Name},
			Spec:     &spec,
		}
		result, err := t.PEToolset.CreateResourceRelease(ctx, args.NamespaceName, body)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetResourceRelease(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_resource_release"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewResourceRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the full definition of a resource release including the embedded ResourceType " +
			"snapshot and captured parameter values.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Resource release name. Use list_resource_releases to discover valid names."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetResourceRelease(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteResourceRelease(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_resource_release"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteResourceRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete a resource release. ResourceReleases are normally cleaned up by the owning " +
			"Resource's finalizer; this is for ad-hoc cleanup of orphaned releases.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Resource release name. Use list_resource_releases to discover valid names."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.DeleteResourceRelease(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}
