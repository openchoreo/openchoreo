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
// Component types — scope-collapsed canonical tools
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterPEListComponentTypes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_component_types", "component type",
		"List component types. With scope=\"namespace\" (default) lists a namespace's component types "+
			"(requires namespace_name); with scope=\"cluster\" lists platform-wide ClusterComponentTypes. "+
			"Component types define the structure and capabilities of components (e.g., WebApplication, Service, "+
			"ScheduledTask). Supports pagination via limit and cursor.",
		authzcore.ActionViewComponentType, authzcore.ActionViewClusterComponentType,
		scopedListHandlers{
			namespace: t.PEToolset.ListComponentTypes,
			cluster:   t.PEToolset.ListClusterComponentTypes,
		})
}

func (t *Toolsets) RegisterListComponentTypes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_component_types", "component type",
		"List component types. With scope=\"namespace\" (default) lists a namespace's component types "+
			"(requires namespace_name); with scope=\"cluster\" lists platform-wide ClusterComponentTypes. "+
			"Component types define the structure and capabilities of components (e.g., WebApplication, Service, "+
			"ScheduledTask). Supports pagination via limit and cursor.",
		authzcore.ActionViewComponentType, authzcore.ActionViewClusterComponentType,
		scopedListHandlers{
			namespace: t.ComponentToolset.ListComponentTypes,
			cluster:   t.ComponentToolset.ListClusterComponentTypes,
		})
}

func (t *Toolsets) RegisterPEGetComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_component_type", "component type",
		"Get the full definition of a component type including its complete spec. Use scope=\"cluster\" for a "+
			"platform-wide ClusterComponentType. Call this before update_component_type to retrieve the current spec.",
		authzcore.ActionViewComponentType, authzcore.ActionViewClusterComponentType,
		"name", "Component type name. Use list_component_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetComponentType,
			cluster:   t.PEToolset.GetClusterComponentType,
		})
}

func (t *Toolsets) RegisterGetComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_component_type", "component type",
		"Get the full definition of a component type including its complete spec. Use scope=\"cluster\" for a "+
			"platform-wide ClusterComponentType. Call this before update_component_type to retrieve the current spec.",
		authzcore.ActionViewComponentType, authzcore.ActionViewClusterComponentType,
		"name", "Component type name. Use list_component_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.ComponentToolset.GetComponentType,
			cluster:   t.ComponentToolset.GetClusterComponentType,
		})
}

func (t *Toolsets) RegisterPEGetComponentTypeSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_component_type_schema", "component type",
		"Get the JSON schema for a component type (required fields, optional fields, and their types). "+
			"Use scope=\"cluster\" for a platform-wide ClusterComponentType.",
		authzcore.ActionViewComponentType, authzcore.ActionViewClusterComponentType,
		"name", "Component type name. Use list_component_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetComponentTypeSchema,
			cluster:   t.PEToolset.GetClusterComponentTypeSchema,
		})
}

func (t *Toolsets) RegisterGetComponentTypeSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_component_type_schema", "component type",
		"Get the JSON schema for a component type (required fields, optional fields, and their types). "+
			"Use scope=\"cluster\" for a platform-wide ClusterComponentType.",
		authzcore.ActionViewComponentType, authzcore.ActionViewClusterComponentType,
		"name", "Component type name. Use list_component_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.ComponentToolset.GetComponentTypeSchema,
			cluster:   t.ComponentToolset.GetClusterComponentTypeSchema,
		})
}

func (t *Toolsets) RegisterGetComponentTypeCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSchemaTool(s, perms, "get_component_type_creation_schema", "component type",
		"Get the spec schema for creating a component type. Use scope=\"namespace\" (default) for a "+
			"namespace-scoped ComponentType or scope=\"cluster\" for a platform-wide ClusterComponentType. "+
			"Call this before create_component_type to understand the spec structure.",
		authzcore.ActionCreateComponentType, authzcore.ActionCreateClusterComponentType,
		scopedSchemaProviders{
			namespace: func() (any, error) { return ComponentTypeCreationSchema() },
			cluster:   func() (any, error) { return ClusterComponentTypeCreationSchema() },
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterCreateComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "create_component_type", "component type",
		"Create a component type. With scope=\"namespace\" (default) it is created in namespace_name; with "+
			"scope=\"cluster\" it is a platform-wide ClusterComponentType available to all namespaces. Component "+
			"types define the structure, workload type, allowed traits, and allowed workflows for components.",
		authzcore.ActionCreateComponentType, authzcore.ActionCreateClusterComponentType,
		"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)",
		"Use get_component_type_creation_schema (with the matching scope) to check the schema",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ComponentTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateComponentType(ctx, ns, &gen.CreateComponentTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ClusterComponentTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateClusterComponentType(ctx, &gen.CreateClusterComponentTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterUpdateComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "update_component_type", "component type",
		"Update an existing component type (full replacement). Use scope=\"cluster\" for a platform-wide "+
			"ClusterComponentType. Use get_component_type to retrieve the current spec first.",
		authzcore.ActionUpdateComponentType, authzcore.ActionUpdateClusterComponentType,
		"Name of the component type to update. Use list_component_types to discover valid names",
		"Full component type spec to replace the existing one. Use get_component_type to retrieve the current spec first.",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ComponentTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateComponentType(ctx, ns, &gen.UpdateComponentTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ClusterComponentTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateClusterComponentType(ctx, &gen.UpdateClusterComponentTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

func (t *Toolsets) RegisterDeleteComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "delete_component_type", "component type",
		"Delete a component type. Use scope=\"cluster\" for a platform-wide ClusterComponentType.",
		authzcore.ActionDeleteComponentType, authzcore.ActionDeleteClusterComponentType,
		"name", "Name of the component type to delete. Use list_component_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.DeleteComponentType,
			cluster:   t.PEToolset.DeleteClusterComponentType,
		})
}
