// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ---------------------------------------------------------------------------
// Project types — scope-collapsed canonical tools
// Reads are dual-registered: ProjectToolset (dev surface) and PEToolset (PE surface).
// Writes and creation schema are PE-only.
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterPEListProjectTypes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_project_types", "project type",
		"List project types. With scope=\"namespace\" (default) lists a namespace's project types "+
			"(requires namespace_name); with scope=\"cluster\" lists platform-wide ClusterProjectTypes. "+
			"Project types define the cell resources, parameters, and per-env config schema that Projects "+
			"reference via spec.type. Supports pagination via limit and cursor.",
		authzcore.ActionViewProjectType, authzcore.ActionViewClusterProjectType,
		scopedListHandlers{
			namespace: t.PEToolset.ListProjectTypes,
			cluster:   t.PEToolset.ListClusterProjectTypes,
		})
}

func (t *Toolsets) RegisterListProjectTypes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_project_types", "project type",
		"List project types. With scope=\"namespace\" (default) lists a namespace's project types "+
			"(requires namespace_name); with scope=\"cluster\" lists platform-wide ClusterProjectTypes. "+
			"Project types define the cell resources, parameters, and per-env config schema that Projects "+
			"reference via spec.type. Supports pagination via limit and cursor.",
		authzcore.ActionViewProjectType, authzcore.ActionViewClusterProjectType,
		scopedListHandlers{
			namespace: t.ProjectToolset.ListProjectTypes,
			cluster:   t.ProjectToolset.ListClusterProjectTypes,
		})
}

func (t *Toolsets) RegisterPEGetProjectType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_project_type", "project type",
		"Get the full definition of a project type including its complete spec. Use scope=\"cluster\" for a "+
			"platform-wide ClusterProjectType. Call this before update_project_type to retrieve the current spec.",
		authzcore.ActionViewProjectType, authzcore.ActionViewClusterProjectType,
		"name", "Project type name. Use list_project_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetProjectType,
			cluster:   t.PEToolset.GetClusterProjectType,
		})
}

func (t *Toolsets) RegisterGetProjectType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_project_type", "project type",
		"Get the full definition of a project type including its complete spec. Use scope=\"cluster\" for a "+
			"platform-wide ClusterProjectType.",
		authzcore.ActionViewProjectType, authzcore.ActionViewClusterProjectType,
		"name", "Project type name. Use list_project_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.ProjectToolset.GetProjectType,
			cluster:   t.ProjectToolset.GetClusterProjectType,
		})
}

func (t *Toolsets) RegisterPEGetProjectTypeSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_project_type_schema", "project type",
		"Get the parameters JSON schema for a project type (the schema that Project.spec.parameters "+
			"must conform to). Use scope=\"cluster\" for a platform-wide ClusterProjectType.",
		authzcore.ActionViewProjectType, authzcore.ActionViewClusterProjectType,
		"name", "Project type name. Use list_project_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetProjectTypeSchema,
			cluster:   t.PEToolset.GetClusterProjectTypeSchema,
		})
}

func (t *Toolsets) RegisterGetProjectTypeSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_project_type_schema", "project type",
		"Get the parameters JSON schema for a project type (the schema that Project.spec.parameters "+
			"must conform to). Use scope=\"cluster\" for a platform-wide ClusterProjectType.",
		authzcore.ActionViewProjectType, authzcore.ActionViewClusterProjectType,
		"name", "Project type name. Use list_project_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.ProjectToolset.GetProjectTypeSchema,
			cluster:   t.ProjectToolset.GetClusterProjectTypeSchema,
		})
}

func (t *Toolsets) RegisterGetProjectTypeCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSchemaTool(s, perms, "get_project_type_creation_schema", "project type",
		"Get the spec schema for creating a project type. Use scope=\"namespace\" (default) for a "+
			"namespace-scoped ProjectType or scope=\"cluster\" for a platform-wide ClusterProjectType. "+
			"Call this before create_project_type to understand the spec structure.",
		authzcore.ActionCreateProjectType, authzcore.ActionCreateClusterProjectType,
		scopedSchemaProviders{
			namespace: func() (any, error) { return ProjectTypeCreationSchema() },
			cluster:   func() (any, error) { return ClusterProjectTypeCreationSchema() },
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterCreateProjectType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "create_project_type", "project type",
		"Create a project type. With scope=\"namespace\" (default) it is created in namespace_name; with "+
			"scope=\"cluster\" it is a platform-wide ClusterProjectType available to all namespaces. Project "+
			"types declare the cell resources, parameters schema, per-env config schema, and validations for "+
			"the Projects that reference them.",
		authzcore.ActionCreateProjectType, authzcore.ActionCreateClusterProjectType,
		"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)",
		"Use get_project_type_creation_schema (with the matching scope) to check the schema",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ProjectTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateProjectType(ctx, ns, &gen.CreateProjectTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ProjectTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateClusterProjectType(ctx, &gen.CreateClusterProjectTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterUpdateProjectType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "update_project_type", "project type",
		"Update an existing project type (full replacement). Use scope=\"cluster\" for a platform-wide "+
			"ClusterProjectType. Use get_project_type to retrieve the current spec first.",
		authzcore.ActionUpdateProjectType, authzcore.ActionUpdateClusterProjectType,
		"Name of the project type to update. Use list_project_types to discover valid names",
		"Full project type spec to replace the existing one. Use get_project_type to retrieve the current spec first.",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ProjectTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateProjectType(ctx, ns, &gen.UpdateProjectTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ProjectTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateClusterProjectType(ctx, &gen.UpdateClusterProjectTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

func (t *Toolsets) RegisterDeleteProjectType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "delete_project_type", "project type",
		"Delete a project type. Use scope=\"cluster\" for a platform-wide ClusterProjectType.",
		authzcore.ActionDeleteProjectType, authzcore.ActionDeleteClusterProjectType,
		"name", "Name of the project type to delete. Use list_project_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.DeleteProjectType,
			cluster:   t.PEToolset.DeleteClusterProjectType,
		})
}
