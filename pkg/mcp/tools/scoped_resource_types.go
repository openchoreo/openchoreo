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
// Resource types — scope-collapsed canonical tools
// Reads are dual-registered: ResourceToolset (dev surface) and PEToolset (PE surface).
// Writes and creation schema are PE-only.
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterPEListResourceTypes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_resource_types", "resource type",
		"List resource types. With scope=\"namespace\" (default) lists a namespace's resource types "+
			"(requires namespace_name); with scope=\"cluster\" lists platform-wide ClusterResourceTypes. "+
			"Resource types define managed-infrastructure templates (databases, queues, caches) that "+
			"Resources reference. Supports pagination via limit and cursor.",
		authzcore.ActionViewResourceType, authzcore.ActionViewClusterResourceType,
		scopedListHandlers{
			namespace: t.PEToolset.ListResourceTypes,
			cluster:   t.PEToolset.ListClusterResourceTypes,
		})
}

func (t *Toolsets) RegisterListResourceTypes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_resource_types", "resource type",
		"List resource types. With scope=\"namespace\" (default) lists a namespace's resource types "+
			"(requires namespace_name); with scope=\"cluster\" lists platform-wide ClusterResourceTypes. "+
			"Resource types define managed-infrastructure templates (databases, queues, caches) that "+
			"Resources reference. Supports pagination via limit and cursor.",
		authzcore.ActionViewResourceType, authzcore.ActionViewClusterResourceType,
		scopedListHandlers{
			namespace: t.ResourceToolset.ListResourceTypes,
			cluster:   t.ResourceToolset.ListClusterResourceTypes,
		})
}

func (t *Toolsets) RegisterPEGetResourceType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_resource_type", "resource type",
		"Get the full definition of a resource type including its complete spec. Use scope=\"cluster\" for a "+
			"platform-wide ClusterResourceType. Call this before update_resource_type to retrieve the current spec.",
		authzcore.ActionViewResourceType, authzcore.ActionViewClusterResourceType,
		"name", "Resource type name. Use list_resource_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetResourceType,
			cluster:   t.PEToolset.GetClusterResourceType,
		})
}

func (t *Toolsets) RegisterGetResourceType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_resource_type", "resource type",
		"Get the full definition of a resource type including its complete spec. Use scope=\"cluster\" for a "+
			"platform-wide ClusterResourceType.",
		authzcore.ActionViewResourceType, authzcore.ActionViewClusterResourceType,
		"name", "Resource type name. Use list_resource_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.ResourceToolset.GetResourceType,
			cluster:   t.ResourceToolset.GetClusterResourceType,
		})
}

func (t *Toolsets) RegisterPEGetResourceTypeSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_resource_type_schema", "resource type",
		"Get the parameters JSON schema for a resource type (the schema that Resource.spec.parameters "+
			"must conform to). Use scope=\"cluster\" for a platform-wide ClusterResourceType.",
		authzcore.ActionViewResourceType, authzcore.ActionViewClusterResourceType,
		"name", "Resource type name. Use list_resource_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetResourceTypeSchema,
			cluster:   t.PEToolset.GetClusterResourceTypeSchema,
		})
}

func (t *Toolsets) RegisterGetResourceTypeSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_resource_type_schema", "resource type",
		"Get the parameters JSON schema for a resource type (the schema that Resource.spec.parameters "+
			"must conform to). Use scope=\"cluster\" for a platform-wide ClusterResourceType.",
		authzcore.ActionViewResourceType, authzcore.ActionViewClusterResourceType,
		"name", "Resource type name. Use list_resource_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.ResourceToolset.GetResourceTypeSchema,
			cluster:   t.ResourceToolset.GetClusterResourceTypeSchema,
		})
}

func (t *Toolsets) RegisterGetResourceTypeCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSchemaTool(s, perms, "get_resource_type_creation_schema", "resource type",
		"Get the spec schema for creating a resource type. Use scope=\"namespace\" (default) for a "+
			"namespace-scoped ResourceType or scope=\"cluster\" for a platform-wide ClusterResourceType. "+
			"Call this before create_resource_type to understand the spec structure.",
		authzcore.ActionCreateResourceType, authzcore.ActionCreateClusterResourceType,
		scopedSchemaProviders{
			namespace: func() (any, error) { return ResourceTypeCreationSchema() },
			cluster:   func() (any, error) { return ClusterResourceTypeCreationSchema() },
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterCreateResourceType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "create_resource_type", "resource type",
		"Create a resource type. With scope=\"namespace\" (default) it is created in namespace_name; with "+
			"scope=\"cluster\" it is a platform-wide ClusterResourceType available to all namespaces. Resource "+
			"types declare the parameters, environment configs, outputs, and provisioned manifests for "+
			"managed infrastructure templates that Resources reference.",
		authzcore.ActionCreateResourceType, authzcore.ActionCreateClusterResourceType,
		"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)",
		"Use get_resource_type_creation_schema (with the matching scope) to check the schema",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ResourceTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateResourceType(ctx, ns, &gen.CreateResourceTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ResourceTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateClusterResourceType(ctx, &gen.CreateClusterResourceTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterUpdateResourceType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "update_resource_type", "resource type",
		"Update an existing resource type (full replacement). Use scope=\"cluster\" for a platform-wide "+
			"ClusterResourceType. Use get_resource_type to retrieve the current spec first.",
		authzcore.ActionUpdateResourceType, authzcore.ActionUpdateClusterResourceType,
		"Name of the resource type to update. Use list_resource_types to discover valid names",
		"Full resource type spec to replace the existing one. Use get_resource_type to retrieve the current spec first.",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ResourceTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateResourceType(ctx, ns, &gen.UpdateResourceTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ResourceTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateClusterResourceType(ctx, &gen.UpdateClusterResourceTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

func (t *Toolsets) RegisterDeleteResourceType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "delete_resource_type", "resource type",
		"Delete a resource type. Use scope=\"cluster\" for a platform-wide ClusterResourceType.",
		authzcore.ActionDeleteResourceType, authzcore.ActionDeleteClusterResourceType,
		"name", "Name of the resource type to delete. Use list_resource_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.DeleteResourceType,
			cluster:   t.PEToolset.DeleteClusterResourceType,
		})
}
