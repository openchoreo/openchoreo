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
// Traits — scope-collapsed canonical tools
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterPEListTraits(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_traits", "trait",
		"List traits. With scope=\"namespace\" (default) lists a namespace's traits (requires namespace_name); "+
			"with scope=\"cluster\" lists platform-wide ClusterTraits. Traits add capabilities to components "+
			"(e.g., autoscaling, ingress, service mesh). Supports pagination via limit and cursor.",
		authzcore.ActionViewTrait, authzcore.ActionViewClusterTrait,
		scopedListHandlers{
			namespace: t.PEToolset.ListTraits,
			cluster:   t.PEToolset.ListClusterTraits,
		})
}

func (t *Toolsets) RegisterListTraits(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_traits", "trait",
		"List traits. With scope=\"namespace\" (default) lists a namespace's traits (requires namespace_name); "+
			"with scope=\"cluster\" lists platform-wide ClusterTraits. Traits add capabilities to components "+
			"(e.g., autoscaling, ingress, service mesh). Supports pagination via limit and cursor.",
		authzcore.ActionViewTrait, authzcore.ActionViewClusterTrait,
		scopedListHandlers{
			namespace: t.ComponentToolset.ListTraits,
			cluster:   t.ComponentToolset.ListClusterTraits,
		})
}

func (t *Toolsets) RegisterPEGetTrait(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_trait", "trait",
		"Get the full definition of a trait including its complete spec. Use scope=\"cluster\" for a "+
			"platform-wide ClusterTrait. Call this before update_trait to retrieve the current spec.",
		authzcore.ActionViewTrait, authzcore.ActionViewClusterTrait,
		"name", "Trait name. Use list_traits to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetTrait,
			cluster:   t.PEToolset.GetClusterTrait,
		})
}

func (t *Toolsets) RegisterGetTrait(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_trait", "trait",
		"Get the full definition of a trait including its complete spec. Use scope=\"cluster\" for a "+
			"platform-wide ClusterTrait. Call this before update_trait to retrieve the current spec.",
		authzcore.ActionViewTrait, authzcore.ActionViewClusterTrait,
		"name", "Trait name. Use list_traits to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.ComponentToolset.GetTrait,
			cluster:   t.ComponentToolset.GetClusterTrait,
		})
}

func (t *Toolsets) RegisterPEGetTraitSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_trait_schema", "trait",
		"Get the JSON schema for a trait (configuration options and parameters). Use scope=\"cluster\" for a "+
			"platform-wide ClusterTrait.",
		authzcore.ActionViewTrait, authzcore.ActionViewClusterTrait,
		"name", "Trait name. Use list_traits to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetTraitSchema,
			cluster:   t.PEToolset.GetClusterTraitSchema,
		})
}

func (t *Toolsets) RegisterGetTraitSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_trait_schema", "trait",
		"Get the JSON schema for a trait (configuration options and parameters). Use scope=\"cluster\" for a "+
			"platform-wide ClusterTrait.",
		authzcore.ActionViewTrait, authzcore.ActionViewClusterTrait,
		"name", "Trait name. Use list_traits to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.ComponentToolset.GetTraitSchema,
			cluster:   t.ComponentToolset.GetClusterTraitSchema,
		})
}

func (t *Toolsets) RegisterGetTraitCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSchemaTool(s, perms, "get_trait_creation_schema", "trait",
		"Get the spec schema for creating a trait. Use scope=\"namespace\" (default) for a namespace-scoped "+
			"Trait or scope=\"cluster\" for a platform-wide ClusterTrait. Call this before create_trait to "+
			"understand the spec structure.",
		authzcore.ActionCreateTrait, authzcore.ActionCreateClusterTrait,
		scopedSchemaProviders{
			namespace: func() (any, error) { return TraitCreationSchema() },
			cluster:   func() (any, error) { return ClusterTraitCreationSchema() },
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterCreateTrait(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "create_trait", "trait",
		"Create a trait. With scope=\"namespace\" (default) it is created in namespace_name; with "+
			"scope=\"cluster\" it is a platform-wide ClusterTrait available to all namespaces. Traits add "+
			"capabilities to components by creating additional Kubernetes resources or patching existing ones "+
			"(e.g., autoscaling, ingress, service mesh).",
		authzcore.ActionCreateTrait, authzcore.ActionCreateClusterTrait,
		"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)",
		"Use get_trait_creation_schema (with the matching scope) to check the schema",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.TraitSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateTrait(ctx, ns, &gen.CreateTraitJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ClusterTraitSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateClusterTrait(ctx, &gen.CreateClusterTraitJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterUpdateTrait(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "update_trait", "trait",
		"Update an existing trait (full replacement). Use scope=\"cluster\" for a platform-wide ClusterTrait. "+
			"Use get_trait to retrieve the current spec first.",
		authzcore.ActionUpdateTrait, authzcore.ActionUpdateClusterTrait,
		"Name of the trait to update. Use list_traits to discover valid names",
		"Full trait spec to replace the existing one. Use get_trait to retrieve the current spec first.",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.TraitSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateTrait(ctx, ns, &gen.UpdateTraitJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ClusterTraitSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateClusterTrait(ctx, &gen.UpdateClusterTraitJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

func (t *Toolsets) RegisterDeleteTrait(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "delete_trait", "trait",
		"Delete a trait. Use scope=\"cluster\" for a platform-wide ClusterTrait.",
		authzcore.ActionDeleteTrait, authzcore.ActionDeleteClusterTrait,
		"name", "Name of the trait to delete. Use list_traits to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.DeleteTrait,
			cluster:   t.PEToolset.DeleteClusterTrait,
		})
}

// ---------------------------------------------------------------------------
// Deprecated cluster-prefixed aliases — traits
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterPEListClusterTraits(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterListTool(s, perms, "list_cluster_traits", "list_traits",
		"Lists platform-wide cluster-scoped traits. Supports pagination via limit and cursor.",
		authzcore.ActionViewClusterTrait, t.PEToolset.ListClusterTraits)
}

func (t *Toolsets) RegisterListClusterTraits(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterListTool(s, perms, "list_cluster_traits", "list_traits",
		"Lists platform-wide cluster-scoped traits. Supports pagination via limit and cursor.",
		authzcore.ActionViewClusterTrait, t.ComponentToolset.ListClusterTraits)
}

func (t *Toolsets) RegisterPEGetClusterTrait(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_trait", "get_trait",
		"Gets the full definition of a platform-wide cluster-scoped trait.",
		authzcore.ActionViewClusterTrait,
		"name", "Cluster trait name. Use list_cluster_traits to discover valid names",
		t.PEToolset.GetClusterTrait)
}

func (t *Toolsets) RegisterGetClusterTrait(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_trait", "get_trait",
		"Gets the full definition of a platform-wide cluster-scoped trait.",
		authzcore.ActionViewClusterTrait,
		"name", "Cluster trait name. Use list_cluster_traits to discover valid names",
		t.ComponentToolset.GetClusterTrait)
}

func (t *Toolsets) RegisterPEGetClusterTraitSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_trait_schema", "get_trait_schema",
		"Gets the JSON schema for a platform-wide cluster-scoped trait.",
		authzcore.ActionViewClusterTrait,
		"name", "Cluster trait name. Use list_cluster_traits to discover valid names",
		t.PEToolset.GetClusterTraitSchema)
}

func (t *Toolsets) RegisterGetClusterTraitSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_trait_schema", "get_trait_schema",
		"Gets the JSON schema for a platform-wide cluster-scoped trait.",
		authzcore.ActionViewClusterTrait,
		"name", "Cluster trait name. Use list_cluster_traits to discover valid names",
		t.ComponentToolset.GetClusterTraitSchema)
}

func (t *Toolsets) RegisterGetClusterTraitCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterSchemaTool(s, perms,
		"get_cluster_trait_creation_schema", "get_trait_creation_schema",
		"Returns the spec schema for creating a platform-wide cluster-scoped trait.",
		authzcore.ActionCreateClusterTrait,
		func() (any, error) { return ClusterTraitCreationSchema() })
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterCreateClusterTrait(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterWriteTool(s, perms, "create_cluster_trait", "create_trait",
		"Creates a platform-wide cluster-scoped trait available to all namespaces.",
		authzcore.ActionCreateClusterTrait,
		"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)",
		"Cluster trait specification defining what resources the trait creates or patches. "+
			"Use get_cluster_trait_schema on an existing trait to see the full structure.",
		func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
			spec, err := buildSpec[gen.ClusterTraitSpec](specRaw)
			if err != nil {
				return nil, err
			}
			return t.PEToolset.CreateClusterTrait(ctx, &gen.CreateClusterTraitJSONRequestBody{
				Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
			})
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterUpdateClusterTrait(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterWriteTool(s, perms, "update_cluster_trait", "update_trait",
		"Updates a platform-wide cluster-scoped trait (full replacement). "+
			"Use get_cluster_trait to retrieve the current definition first.",
		authzcore.ActionUpdateClusterTrait,
		"Name of the cluster trait to update. Use list_cluster_traits to discover valid names",
		"Full cluster trait spec to replace the existing one. Use get_cluster_trait to retrieve the current spec first.",
		func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
			spec, err := buildSpec[gen.ClusterTraitSpec](specRaw)
			if err != nil {
				return nil, err
			}
			return t.PEToolset.UpdateClusterTrait(ctx, &gen.UpdateClusterTraitJSONRequestBody{
				Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
			})
		})
}

func (t *Toolsets) RegisterDeleteClusterTrait(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "delete_cluster_trait", "delete_trait",
		"Deletes a platform-wide cluster-scoped trait.",
		authzcore.ActionDeleteClusterTrait,
		"name", "Name of the cluster trait to delete. Use list_cluster_traits to discover valid names",
		t.PEToolset.DeleteClusterTrait)
}
