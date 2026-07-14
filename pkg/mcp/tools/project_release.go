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
func (t *Toolsets) RegisterPEListProjectReleases(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_project_releases"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewProjectRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all releases for a project. ProjectReleases are immutable snapshots of " +
			"Project.spec plus the referenced (Cluster)ProjectType.spec at the time they were cut. " +
			"Created automatically by the Project controller; rarely created by hand. Supports " +
			"pagination via limit and cursor.",
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
		result, err := t.PEToolset.ListProjectReleases(
			ctx, args.NamespaceName, args.ProjectName,
			ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPECreateProjectRelease(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_project_release"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateProjectRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a ProjectRelease (immutable snapshot of Project.spec + the referenced " +
			"(Cluster)ProjectType.spec). Normally created by the Project controller; expose this for " +
			"GitOps-style manual snapshots. The type_spec argument must contain the full ProjectType " +
			"snapshot to embed in the release.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Release name. Convention: {project}-{hash}."),
			"project_name":   stringProperty("Name of the project that owns the release."),
			"type_name":      stringProperty("Name of the (Cluster)ProjectType the snapshot was taken from."),
			"type_kind": stringProperty(
				"Optional: \"ProjectType\" (default, namespace-scoped) or \"ClusterProjectType\" (cluster-scoped)."),
			"type_spec": map[string]any{
				"type":        "object",
				"description": "Full ProjectTypeSpec snapshot to embed in the release.",
			},
			"parameters": map[string]any{
				"type":        "object",
				"description": "Optional: parameter values captured from the source Project at release time.",
			},
		}, []string{"namespace_name", "name", "project_name", "type_name", "type_spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		Name          string                 `json:"name"`
		ProjectName   string                 `json:"project_name"`
		TypeName      string                 `json:"type_name"`
		TypeKind      string                 `json:"type_kind"`
		TypeSpec      map[string]interface{} `json:"type_spec"`
		Parameters    map[string]interface{} `json:"parameters"`
	}) (*mcp.CallToolResult, any, error) {
		typeSpec, err := buildSpec[gen.ProjectTypeSpec](args.TypeSpec)
		if err != nil {
			return nil, nil, err
		}
		kind := gen.ProjectReleaseSpecProjectTypeKind(args.TypeKind)
		if args.TypeKind == "" {
			kind = gen.ProjectReleaseSpecProjectTypeKindProjectType
		}
		spec := gen.ProjectReleaseSpec{
			Owner: struct {
				ProjectName string `json:"projectName"`
			}{
				ProjectName: args.ProjectName,
			},
			ProjectType: struct {
				Kind gen.ProjectReleaseSpecProjectTypeKind `json:"kind"`
				Name string                                `json:"name"`
				Spec gen.ProjectTypeSpec                   `json:"spec"`
			}{
				Kind: kind,
				Name: args.TypeName,
				Spec: *typeSpec,
			},
		}
		if args.Parameters != nil {
			spec.Parameters = &args.Parameters
		}
		body := &gen.CreateProjectReleaseJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: args.Name},
			Spec:     &spec,
		}
		result, err := t.PEToolset.CreateProjectRelease(ctx, args.NamespaceName, body)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetProjectRelease(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_project_release"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewProjectRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the full definition of a project release including the embedded ProjectType " +
			"snapshot and captured parameter values.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Project release name. Use list_project_releases to discover valid names."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetProjectRelease(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteProjectRelease(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_project_release"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteProjectRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete a project release. ProjectReleases are normally cleaned up by the owning " +
			"Project's finalizer; this is for ad-hoc cleanup of orphaned releases.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Project release name. Use list_project_releases to discover valid names."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.DeleteProjectRelease(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}
