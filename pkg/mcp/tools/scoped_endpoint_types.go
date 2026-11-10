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
// Endpoint types — scope-collapsed canonical tools
// Reads are dual-registered: EndpointToolset (dev surface) and PEToolset (PE surface).
// Writes and creation schema are PE-only.
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterPEListEndpointTypes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_endpoint_types", "endpoint type",
		"List endpoint types. With scope=\"namespace\" (default) lists a namespace's endpoint types "+
			"(requires namespace_name); with scope=\"cluster\" lists platform-wide ClusterEndpointTypes. "+
			"Endpoint types define routing templates (e.g. an HTTPRoute per environment) that "+
			"Endpoints reference. Supports pagination via limit and cursor.",
		authzcore.ActionViewEndpointType, authzcore.ActionViewClusterEndpointType,
		scopedListHandlers{
			namespace: t.PEToolset.ListEndpointTypes,
			cluster:   t.PEToolset.ListClusterEndpointTypes,
		})
}

func (t *Toolsets) RegisterListEndpointTypes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_endpoint_types", "endpoint type",
		"List endpoint types. With scope=\"namespace\" (default) lists a namespace's endpoint types "+
			"(requires namespace_name); with scope=\"cluster\" lists platform-wide ClusterEndpointTypes. "+
			"Endpoint types define routing templates (e.g. an HTTPRoute per environment) that "+
			"Endpoints reference. Supports pagination via limit and cursor.",
		authzcore.ActionViewEndpointType, authzcore.ActionViewClusterEndpointType,
		scopedListHandlers{
			namespace: t.EndpointToolset.ListEndpointTypes,
			cluster:   t.EndpointToolset.ListClusterEndpointTypes,
		})
}

func (t *Toolsets) RegisterPEGetEndpointType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_endpoint_type", "endpoint type",
		"Get the full definition of an endpoint type including its complete spec. Use scope=\"cluster\" for a "+
			"platform-wide ClusterEndpointType. Call this before update_endpoint_type to retrieve the current spec.",
		authzcore.ActionViewEndpointType, authzcore.ActionViewClusterEndpointType,
		"name", "Endpoint type name. Use list_endpoint_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetEndpointType,
			cluster:   t.PEToolset.GetClusterEndpointType,
		})
}

func (t *Toolsets) RegisterGetEndpointType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_endpoint_type", "endpoint type",
		"Get the full definition of an endpoint type including its complete spec. Use scope=\"cluster\" for a "+
			"platform-wide ClusterEndpointType.",
		authzcore.ActionViewEndpointType, authzcore.ActionViewClusterEndpointType,
		"name", "Endpoint type name. Use list_endpoint_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.EndpointToolset.GetEndpointType,
			cluster:   t.EndpointToolset.GetClusterEndpointType,
		})
}

func (t *Toolsets) RegisterPEGetEndpointTypeSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_endpoint_type_schema", "endpoint type",
		"Get the parameters JSON schema for an endpoint type (the schema that Endpoint.spec.parameters "+
			"must conform to). Use scope=\"cluster\" for a platform-wide ClusterEndpointType.",
		authzcore.ActionViewEndpointType, authzcore.ActionViewClusterEndpointType,
		"name", "Endpoint type name. Use list_endpoint_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetEndpointTypeSchema,
			cluster:   t.PEToolset.GetClusterEndpointTypeSchema,
		})
}

func (t *Toolsets) RegisterGetEndpointTypeSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_endpoint_type_schema", "endpoint type",
		"Get the parameters JSON schema for an endpoint type (the schema that Endpoint.spec.parameters "+
			"must conform to). Use scope=\"cluster\" for a platform-wide ClusterEndpointType.",
		authzcore.ActionViewEndpointType, authzcore.ActionViewClusterEndpointType,
		"name", "Endpoint type name. Use list_endpoint_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.EndpointToolset.GetEndpointTypeSchema,
			cluster:   t.EndpointToolset.GetClusterEndpointTypeSchema,
		})
}

func (t *Toolsets) RegisterGetEndpointTypeCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSchemaTool(s, perms, "get_endpoint_type_creation_schema", "endpoint type",
		"Get the spec schema for creating an endpoint type. Use scope=\"namespace\" (default) for a "+
			"namespace-scoped EndpointType or scope=\"cluster\" for a platform-wide ClusterEndpointType. "+
			"Call this before create_endpoint_type to understand the spec structure.",
		authzcore.ActionCreateEndpointType, authzcore.ActionCreateClusterEndpointType,
		scopedSchemaProviders{
			namespace: func() (any, error) { return EndpointTypeCreationSchema() },
			cluster:   func() (any, error) { return ClusterEndpointTypeCreationSchema() },
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterCreateEndpointType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "create_endpoint_type", "endpoint type",
		"Create an endpoint type. With scope=\"namespace\" (default) it is created in namespace_name; with "+
			"scope=\"cluster\" it is a platform-wide ClusterEndpointType available to all namespaces. Endpoint "+
			"types declare the parameters, environment configs, validations, and routing manifests "+
			"that Endpoints render through.",
		authzcore.ActionCreateEndpointType, authzcore.ActionCreateClusterEndpointType,
		"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)",
		"Use get_endpoint_type_creation_schema (with the matching scope) to check the schema",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.EndpointTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateEndpointType(ctx, ns, &gen.CreateEndpointTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.EndpointTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateClusterEndpointType(ctx, &gen.CreateClusterEndpointTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterUpdateEndpointType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "update_endpoint_type", "endpoint type",
		"Update an existing endpoint type (full replacement). Use scope=\"cluster\" for a platform-wide "+
			"ClusterEndpointType. Use get_endpoint_type to retrieve the current spec first.",
		authzcore.ActionUpdateEndpointType, authzcore.ActionUpdateClusterEndpointType,
		"Name of the endpoint type to update. Use list_endpoint_types to discover valid names",
		"Full endpoint type spec to replace the existing one. Use get_endpoint_type to retrieve the current spec first.",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.EndpointTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateEndpointType(ctx, ns, &gen.UpdateEndpointTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.EndpointTypeSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateClusterEndpointType(ctx, &gen.UpdateClusterEndpointTypeJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

func (t *Toolsets) RegisterDeleteEndpointType(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "delete_endpoint_type", "endpoint type",
		"Delete an endpoint type. Use scope=\"cluster\" for a platform-wide ClusterEndpointType.",
		authzcore.ActionDeleteEndpointType, authzcore.ActionDeleteClusterEndpointType,
		"name", "Name of the endpoint type to delete. Use list_endpoint_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.DeleteEndpointType,
			cluster:   t.PEToolset.DeleteClusterEndpointType,
		})
}
