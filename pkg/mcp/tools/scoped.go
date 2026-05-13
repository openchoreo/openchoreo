// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ---------------------------------------------------------------------------
// Scope-collapsed tools
//
// A scope-collapsed tool replaces a namespace-scoped tool and its cluster-scoped
// counterpart (e.g. get_component_type / get_cluster_component_type) with a single
// tool that selects behavior via a `scope` argument. The cluster-prefixed names
// remain registered as deprecated compatibility aliases; they are hidden from the
// default tools/list response (use ?includeDeprecatedTools=true to see them) and
// every result they return carries a deprecation warning.
//
// Authorization: a scope-collapsed tool declares one authz action per scope via
// ToolPermission.ScopedActions. tools/list shows it when the user holds at least
// one of those actions; tools/call resolves the required action from the `scope`
// argument. This mirrors the authorization of the OpenChoreo CLI `apply` command,
// which routes to the namespace or cluster service implementation based on the
// resource it is given.
// ---------------------------------------------------------------------------

// scopedActions builds the ScopedActions map for a scope-collapsed tool.
func scopedActions(nsAction, clusterAction string) map[string]string {
	return map[string]string{ScopeNamespace: nsAction, ScopeCluster: clusterAction}
}

// unmarshalArgs decodes the raw tool arguments into v, tolerating empty input.
func unmarshalArgs(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, v)
}

// stringArg returns the string value at key in m, or "" when absent or not a string.
func stringArg(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// ---------------------------------------------------------------------------
// Generic registration helpers — scope-collapsed canonical tools
// ---------------------------------------------------------------------------

// scopedListHandlers carries the per-scope implementations of a scope-collapsed
// list tool.
type scopedListHandlers struct {
	namespace func(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	cluster   func(ctx context.Context, opts ListOpts) (any, error)
}

// registerScopedListTool registers a scope-collapsed list tool. resourceNoun is a
// short human-readable name for the listed resource (e.g. "component type").
func registerScopedListTool(
	s *mcp.Server, perms map[string]ToolPermission,
	name, resourceNoun, description, nsAction, clusterAction string,
	h scopedListHandlers,
) {
	perms[name] = ToolPermission{ToolName: name, ScopedActions: scopedActions(nsAction, clusterAction)}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: description,
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"scope":          scopeProperty(resourceNoun),
			"namespace_name": scopedNamespaceProperty(),
		}), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, raw json.RawMessage) (*mcp.CallToolResult, any, error) {
		var args struct {
			Scope         string `json:"scope"`
			NamespaceName string `json:"namespace_name"`
			Limit         int    `json:"limit"`
			Cursor        string `json:"cursor"`
		}
		if err := unmarshalArgs(raw, &args); err != nil {
			return handleToolResult(nil, err)
		}
		scope, err := resolveScope(args.Scope)
		if err != nil {
			return handleToolResult(nil, err)
		}
		if err := requireNamespaceForScope(scope, args.NamespaceName); err != nil {
			return handleToolResult(nil, err)
		}
		opts := ListOpts{Limit: args.Limit, Cursor: args.Cursor}
		if scope == ScopeCluster {
			return handleToolResult(h.cluster(ctx, opts))
		}
		return handleToolResult(h.namespace(ctx, args.NamespaceName, opts))
	})
}

// scopedSingleResourceHandlers carries the per-scope implementations of a
// scope-collapsed tool that operates on a single named resource (get / get-schema
// / delete).
type scopedSingleResourceHandlers struct {
	namespace func(ctx context.Context, namespaceName, resourceName string) (any, error)
	cluster   func(ctx context.Context, resourceName string) (any, error)
}

// registerScopedSingleResourceTool registers a scope-collapsed tool that takes a
// single resource-name argument (named by nameParam) plus scope/namespace_name.
// nameParam is "name" for every current resource but is kept explicit alongside
// nameParamDesc so a future resource can use a different key.
//
//nolint:unparam // see comment above re: nameParam
func registerScopedSingleResourceTool(
	s *mcp.Server, perms map[string]ToolPermission,
	name, resourceNoun, description, nsAction, clusterAction, nameParam, nameParamDesc string,
	h scopedSingleResourceHandlers,
) {
	perms[name] = ToolPermission{ToolName: name, ScopedActions: scopedActions(nsAction, clusterAction)}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: description,
		InputSchema: createSchema(map[string]any{
			"scope":          scopeProperty(resourceNoun),
			"namespace_name": scopedNamespaceProperty(),
			nameParam:        stringProperty(nameParamDesc),
		}, []string{nameParam}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, raw json.RawMessage) (*mcp.CallToolResult, any, error) {
		var args map[string]any
		if err := unmarshalArgs(raw, &args); err != nil {
			return handleToolResult(nil, err)
		}
		scope, err := resolveScope(stringArg(args, "scope"))
		if err != nil {
			return handleToolResult(nil, err)
		}
		namespaceName := stringArg(args, "namespace_name")
		if err := requireNamespaceForScope(scope, namespaceName); err != nil {
			return handleToolResult(nil, err)
		}
		resourceName := stringArg(args, nameParam)
		if scope == ScopeCluster {
			return handleToolResult(h.cluster(ctx, resourceName))
		}
		return handleToolResult(h.namespace(ctx, namespaceName, resourceName))
	})
}

// scopedSchemaProviders carries the per-scope static schema providers of a
// scope-collapsed creation-schema tool.
type scopedSchemaProviders struct {
	namespace func() (any, error)
	cluster   func() (any, error)
}

// registerScopedSchemaTool registers a scope-collapsed tool that returns a static
// schema and takes only the scope argument.
func registerScopedSchemaTool(
	s *mcp.Server, perms map[string]ToolPermission,
	name, resourceNoun, description, nsAction, clusterAction string,
	h scopedSchemaProviders,
) {
	perms[name] = ToolPermission{ToolName: name, ScopedActions: scopedActions(nsAction, clusterAction)}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: description,
		InputSchema: createSchema(map[string]any{
			"scope": scopeProperty(resourceNoun),
		}, nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, raw json.RawMessage) (*mcp.CallToolResult, any, error) {
		var args struct {
			Scope string `json:"scope"`
		}
		if err := unmarshalArgs(raw, &args); err != nil {
			return handleToolResult(nil, err)
		}
		scope, err := resolveScope(args.Scope)
		if err != nil {
			return handleToolResult(nil, err)
		}
		if scope == ScopeCluster {
			return handleToolResult(h.cluster())
		}
		return handleToolResult(h.namespace())
	})
}

// scopedWriteHandlers carries the per-scope implementations of a scope-collapsed
// create/update tool. The implementation receives the resource name, the assembled
// metadata annotations, and the raw spec object; it is responsible for decoding the
// spec into the scope-appropriate type and calling the service layer.
type scopedWriteHandlers struct {
	namespace func(
		ctx context.Context, namespaceName, name string,
		annotations map[string]string, specRaw map[string]any,
	) (any, error)
	cluster func(
		ctx context.Context, name string,
		annotations map[string]string, specRaw map[string]any,
	) (any, error)
}

// registerScopedWriteTool registers a scope-collapsed create/update tool.
func registerScopedWriteTool(
	s *mcp.Server, perms map[string]ToolPermission,
	name, resourceNoun, description, nsAction, clusterAction, nameParamDesc, specDesc string,
	h scopedWriteHandlers,
) {
	perms[name] = ToolPermission{ToolName: name, ScopedActions: scopedActions(nsAction, clusterAction)}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: description,
		InputSchema: createSchema(map[string]any{
			"scope":          scopeProperty(resourceNoun),
			"namespace_name": scopedNamespaceProperty(),
			"name":           stringProperty(nameParamDesc),
			"display_name":   stringProperty("Human-readable display name"),
			"description":    stringProperty("Human-readable description"),
			"spec": map[string]any{
				"type":        "object",
				"description": specDesc,
			},
		}, []string{"name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, raw json.RawMessage) (*mcp.CallToolResult, any, error) {
		var args struct {
			Scope         string                 `json:"scope"`
			NamespaceName string                 `json:"namespace_name"`
			Name          string                 `json:"name"`
			DisplayName   string                 `json:"display_name"`
			Description   string                 `json:"description"`
			Spec          map[string]interface{} `json:"spec"`
		}
		if err := unmarshalArgs(raw, &args); err != nil {
			return handleToolResult(nil, err)
		}
		scope, err := resolveScope(args.Scope)
		if err != nil {
			return handleToolResult(nil, err)
		}
		if err := requireNamespaceForScope(scope, args.NamespaceName); err != nil {
			return handleToolResult(nil, err)
		}
		annotations := buildAnnotations(args.DisplayName, args.Description)
		if scope == ScopeCluster {
			return handleToolResult(h.cluster(ctx, args.Name, annotations, args.Spec))
		}
		return handleToolResult(h.namespace(ctx, args.NamespaceName, args.Name, annotations, args.Spec))
	})
}

// ---------------------------------------------------------------------------
// Generic registration helpers — deprecated cluster-prefixed aliases
// ---------------------------------------------------------------------------

// Deprecation lifecycle versions for the cluster-prefixed compatibility aliases.
// Update these when the schedule shifts and every alias description / _meta
// follows automatically.
const (
	// deprecatedHiddenInVersion is the release in which the aliases are hidden
	// from the default tools/list response (still callable for that cycle).
	deprecatedHiddenInVersion = "v1.2"
	// deprecatedRemovedInVersion is the release in which the aliases are
	// removed entirely (calls return a not-found error).
	deprecatedRemovedInVersion = "v1.3"
	// deprecatedMetaPrefix is the reverse-DNS prefix used on the structured
	// _meta keys that mark a tool as deprecated. Matches the project's
	// annotation namespace.
	deprecatedMetaPrefix = "openchoreo.dev/"
)

// deprecatedBanner returns the description prefix advertised on every
// deprecated cluster-prefixed alias. It tells the agent which canonical tool
// replaces this alias and when the alias will disappear, so the LLM can plan
// the migration without an out-of-band changelog read.
func deprecatedBanner(canonicalName string) string {
	return fmt.Sprintf(
		"[DEPRECATED — use %s with scope:%q; hidden in %s, removed in %s] ",
		canonicalName, ScopeCluster, deprecatedHiddenInVersion, deprecatedRemovedInVersion,
	)
}

// deprecatedDescription prepends deprecatedBanner to the tool's body description.
func deprecatedDescription(canonicalName, body string) string {
	return deprecatedBanner(canonicalName) + body
}

// deprecatedMeta returns the structured _meta map attached to every deprecated
// cluster-prefixed alias so MCP clients that can introspect _meta (rather than
// parse the description string) can detect the deprecation programmatically.
// Keys use the project's reverse-DNS prefix per the MCP _meta convention.
func deprecatedMeta(canonicalName string) mcp.Meta {
	return mcp.Meta{
		deprecatedMetaPrefix + "deprecated": true,
		deprecatedMetaPrefix + "replacement": map[string]any{
			"tool":  canonicalName,
			"scope": ScopeCluster,
		},
		deprecatedMetaPrefix + "hidden_in":  deprecatedHiddenInVersion,
		deprecatedMetaPrefix + "removed_in": deprecatedRemovedInVersion,
	}
}

// registerDeprecatedClusterListTool registers a deprecated cluster-prefixed list
// alias that routes to the cluster behavior of the named canonical tool. The
// descriptionBody is prepended with the standard deprecation banner.
func registerDeprecatedClusterListTool(
	s *mcp.Server, perms map[string]ToolPermission,
	name, canonicalName, descriptionBody, action string,
	list func(ctx context.Context, opts ListOpts) (any, error),
) {
	perms[name] = ToolPermission{ToolName: name, Action: action}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: deprecatedDescription(canonicalName, descriptionBody),
		Meta:        deprecatedMeta(canonicalName),
		InputSchema: createSchema(addPaginationProperties(map[string]any{}), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Limit  int    `json:"limit,omitempty"`
		Cursor string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := list(ctx, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleDeprecatedToolResult(name, canonicalName, result, err)
	})
}

// registerDeprecatedClusterResourceTool registers a deprecated cluster-prefixed
// alias that takes a single resource-name argument (get / get-schema / delete).
// nameParam is "name" for every current resource but is kept explicit alongside
// nameParamDesc so a future resource can use a different key.
//
//nolint:unparam // see comment above re: nameParam
func registerDeprecatedClusterResourceTool(
	s *mcp.Server, perms map[string]ToolPermission,
	name, canonicalName, descriptionBody, action, nameParam, nameParamDesc string,
	call func(ctx context.Context, resourceName string) (any, error),
) {
	perms[name] = ToolPermission{ToolName: name, Action: action}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: deprecatedDescription(canonicalName, descriptionBody),
		Meta:        deprecatedMeta(canonicalName),
		InputSchema: createSchema(map[string]any{
			nameParam: stringProperty(nameParamDesc),
		}, []string{nameParam}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, raw json.RawMessage) (*mcp.CallToolResult, any, error) {
		var args map[string]any
		if err := unmarshalArgs(raw, &args); err != nil {
			return handleToolResult(nil, err)
		}
		result, err := call(ctx, stringArg(args, nameParam))
		return handleDeprecatedToolResult(name, canonicalName, result, err)
	})
}

// registerDeprecatedClusterSchemaTool registers a deprecated cluster-prefixed
// alias that returns a static schema and takes no arguments.
func registerDeprecatedClusterSchemaTool(
	s *mcp.Server, perms map[string]ToolPermission,
	name, canonicalName, descriptionBody, action string,
	provide func() (any, error),
) {
	perms[name] = ToolPermission{ToolName: name, Action: action}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: deprecatedDescription(canonicalName, descriptionBody),
		Meta:        deprecatedMeta(canonicalName),
		InputSchema: createSchema(map[string]any{}, nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		result, err := provide()
		return handleDeprecatedToolResult(name, canonicalName, result, err)
	})
}

// registerDeprecatedClusterWriteTool registers a deprecated cluster-prefixed
// create/update alias.
func registerDeprecatedClusterWriteTool(
	s *mcp.Server, perms map[string]ToolPermission,
	name, canonicalName, descriptionBody, action, nameParamDesc, specDesc string,
	apply func(ctx context.Context, name string, annotations map[string]string, specRaw map[string]any) (any, error),
) {
	perms[name] = ToolPermission{ToolName: name, Action: action}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: deprecatedDescription(canonicalName, descriptionBody),
		Meta:        deprecatedMeta(canonicalName),
		InputSchema: createSchema(map[string]any{
			"name":         stringProperty(nameParamDesc),
			"display_name": stringProperty("Human-readable display name"),
			"description":  stringProperty("Human-readable description"),
			"spec": map[string]any{
				"type":        "object",
				"description": specDesc,
			},
		}, []string{"name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name        string                 `json:"name"`
		DisplayName string                 `json:"display_name"`
		Description string                 `json:"description"`
		Spec        map[string]interface{} `json:"spec"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := buildAnnotations(args.DisplayName, args.Description)
		result, err := apply(ctx, args.Name, annotations, args.Spec)
		return handleDeprecatedToolResult(name, canonicalName, result, err)
	})
}

// ---------------------------------------------------------------------------
// Component types — scope-collapsed canonical tools
// ---------------------------------------------------------------------------

const (
	scopedComponentTypeNoun = "component type"
	scopedTraitNoun         = "trait"
	scopedWorkflowNoun      = "workflow"
)

func (t *Toolsets) RegisterPEListComponentTypes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_component_types", scopedComponentTypeNoun,
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
	registerScopedListTool(s, perms, "list_component_types", scopedComponentTypeNoun,
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
	registerScopedSingleResourceTool(s, perms, "get_component_type", scopedComponentTypeNoun,
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
	registerScopedSingleResourceTool(s, perms, "get_component_type", scopedComponentTypeNoun,
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
	registerScopedSingleResourceTool(s, perms, "get_component_type_schema", scopedComponentTypeNoun,
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
	registerScopedSingleResourceTool(s, perms, "get_component_type_schema", scopedComponentTypeNoun,
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
	registerScopedSchemaTool(s, perms, "get_component_type_creation_schema", scopedComponentTypeNoun,
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
	registerScopedWriteTool(s, perms, "create_component_type", scopedComponentTypeNoun,
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
	registerScopedWriteTool(s, perms, "update_component_type", scopedComponentTypeNoun,
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
	registerScopedSingleResourceTool(s, perms, "delete_component_type", scopedComponentTypeNoun,
		"Delete a component type. Use scope=\"cluster\" for a platform-wide ClusterComponentType.",
		authzcore.ActionDeleteComponentType, authzcore.ActionDeleteClusterComponentType,
		"name", "Name of the component type to delete. Use list_component_types to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.DeleteComponentType,
			cluster:   t.PEToolset.DeleteClusterComponentType,
		})
}

// ---------------------------------------------------------------------------
// Traits — scope-collapsed canonical tools
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterPEListTraits(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_traits", scopedTraitNoun,
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
	registerScopedListTool(s, perms, "list_traits", scopedTraitNoun,
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
	registerScopedSingleResourceTool(s, perms, "get_trait", scopedTraitNoun,
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
	registerScopedSingleResourceTool(s, perms, "get_trait", scopedTraitNoun,
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
	registerScopedSingleResourceTool(s, perms, "get_trait_schema", scopedTraitNoun,
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
	registerScopedSingleResourceTool(s, perms, "get_trait_schema", scopedTraitNoun,
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
	registerScopedSchemaTool(s, perms, "get_trait_creation_schema", scopedTraitNoun,
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
	registerScopedWriteTool(s, perms, "create_trait", scopedTraitNoun,
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
	registerScopedWriteTool(s, perms, "update_trait", scopedTraitNoun,
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
	registerScopedSingleResourceTool(s, perms, "delete_trait", scopedTraitNoun,
		"Delete a trait. Use scope=\"cluster\" for a platform-wide ClusterTrait.",
		authzcore.ActionDeleteTrait, authzcore.ActionDeleteClusterTrait,
		"name", "Name of the trait to delete. Use list_traits to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.DeleteTrait,
			cluster:   t.PEToolset.DeleteClusterTrait,
		})
}

// ---------------------------------------------------------------------------
// Workflows — scope-collapsed canonical tools
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterPEListWorkflows(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_workflows", scopedWorkflowNoun,
		"List workflows. With scope=\"namespace\" (default) lists a namespace's workflows (requires "+
			"namespace_name); with scope=\"cluster\" lists platform-wide ClusterWorkflows. Workflows are reusable "+
			"templates that define automated processes such as CI/CD pipelines executed on the workflow plane. "+
			"Supports pagination via limit and cursor.",
		authzcore.ActionViewWorkflow, authzcore.ActionViewClusterWorkflow,
		scopedListHandlers{
			namespace: t.PEToolset.ListWorkflows,
			cluster:   t.PEToolset.ListClusterWorkflows,
		})
}

func (t *Toolsets) RegisterListWorkflows(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_workflows", scopedWorkflowNoun,
		"List workflows. With scope=\"namespace\" (default) lists a namespace's workflows (requires "+
			"namespace_name); with scope=\"cluster\" lists platform-wide ClusterWorkflows. Workflows are reusable "+
			"templates that define automated processes such as CI/CD pipelines executed on the workflow plane. "+
			"Supports pagination via limit and cursor.",
		authzcore.ActionViewWorkflow, authzcore.ActionViewClusterWorkflow,
		scopedListHandlers{
			namespace: t.BuildToolset.ListWorkflows,
			cluster:   t.BuildToolset.ListClusterWorkflows,
		})
}

func (t *Toolsets) RegisterPEGetWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_workflow", scopedWorkflowNoun,
		"Get the full definition of a workflow including its complete spec. Use scope=\"cluster\" for a "+
			"platform-wide ClusterWorkflow. Call this before update_workflow to retrieve the current spec.",
		authzcore.ActionViewWorkflow, authzcore.ActionViewClusterWorkflow,
		"name", "Name of the workflow. Use list_workflows to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetWorkflow,
			cluster:   t.PEToolset.GetClusterWorkflow,
		})
}

func (t *Toolsets) RegisterGetWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_workflow", scopedWorkflowNoun,
		"Get the full definition of a workflow including its complete spec. Use scope=\"cluster\" for a "+
			"platform-wide ClusterWorkflow. Call this before update_workflow to retrieve the current spec.",
		authzcore.ActionViewWorkflow, authzcore.ActionViewClusterWorkflow,
		"name", "Name of the workflow. Use list_workflows to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.BuildToolset.GetWorkflow,
			cluster:   t.BuildToolset.GetClusterWorkflow,
		})
}

func (t *Toolsets) RegisterPEGetWorkflowSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_workflow_schema", scopedWorkflowNoun,
		"Get the parameter schema for a workflow. Use this to inspect what parameters a workflow accepts "+
			"before configuring a component's workflow field or triggering a workflow run. Use scope=\"cluster\" "+
			"for a platform-wide ClusterWorkflow.",
		authzcore.ActionViewWorkflow, authzcore.ActionViewClusterWorkflow,
		"name", "Name of the workflow. Use list_workflows to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetWorkflowSchema,
			cluster:   t.PEToolset.GetClusterWorkflowSchema,
		})
}

func (t *Toolsets) RegisterGetWorkflowSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_workflow_schema", scopedWorkflowNoun,
		"Get the parameter schema for a workflow. Use this to inspect what parameters a workflow accepts "+
			"before configuring a component's workflow field or triggering a workflow run. Use scope=\"cluster\" "+
			"for a platform-wide ClusterWorkflow.",
		authzcore.ActionViewWorkflow, authzcore.ActionViewClusterWorkflow,
		"name", "Name of the workflow. Use list_workflows to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.BuildToolset.GetWorkflowSchema,
			cluster:   t.BuildToolset.GetClusterWorkflowSchema,
		})
}

func (t *Toolsets) RegisterGetWorkflowCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSchemaTool(s, perms, "get_workflow_creation_schema", scopedWorkflowNoun,
		"Get the spec schema for creating a workflow (runTemplate, parameters definition, repository defaults, "+
			"etc.). Use scope=\"namespace\" (default) for a namespace-scoped Workflow or scope=\"cluster\" for a "+
			"platform-wide ClusterWorkflow. Call this before create_workflow to understand the spec structure.",
		authzcore.ActionCreateWorkflow, authzcore.ActionCreateClusterWorkflow,
		scopedSchemaProviders{
			namespace: func() (any, error) { return WorkflowCreationSchema() },
			cluster:   func() (any, error) { return ClusterWorkflowCreationSchema() },
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterPECreateWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "create_workflow", scopedWorkflowNoun,
		"Create a workflow. With scope=\"namespace\" (default) it is created in namespace_name; with "+
			"scope=\"cluster\" it is a platform-wide ClusterWorkflow available to all namespaces. Workflows are "+
			"reusable CI/CD pipeline templates that execute on the workflow plane.",
		authzcore.ActionCreateWorkflow, authzcore.ActionCreateClusterWorkflow,
		"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)",
		"Workflow specification. Required field: runTemplate (Argo Workflow template definition). "+
			"Use get_workflow_creation_schema (with the matching scope) to check the schema.",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.WorkflowSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateWorkflow(ctx, ns, &gen.CreateWorkflowJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ClusterWorkflowSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateClusterWorkflow(ctx, &gen.CreateClusterWorkflowJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterPEUpdateWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "update_workflow", scopedWorkflowNoun,
		"Update an existing workflow (full replacement). Use scope=\"cluster\" for a platform-wide "+
			"ClusterWorkflow. Use get_workflow to retrieve the current spec first.",
		authzcore.ActionUpdateWorkflow, authzcore.ActionUpdateClusterWorkflow,
		"Name of the workflow to update. Use list_workflows to discover valid names",
		"Full workflow spec to replace the existing one. Use get_workflow to retrieve the current spec first.",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.WorkflowSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateWorkflow(ctx, ns, &gen.UpdateWorkflowJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ClusterWorkflowSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateClusterWorkflow(ctx, &gen.UpdateClusterWorkflowJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

func (t *Toolsets) RegisterPEDeleteWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "delete_workflow", scopedWorkflowNoun,
		"Delete a workflow. Use scope=\"cluster\" for a platform-wide ClusterWorkflow.",
		authzcore.ActionDeleteWorkflow, authzcore.ActionDeleteClusterWorkflow,
		"name", "Name of the workflow to delete. Use list_workflows to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.DeleteWorkflow,
			cluster:   t.PEToolset.DeleteClusterWorkflow,
		})
}

// ---------------------------------------------------------------------------
// Plane resources (data plane / workflow plane / observability plane)
// — scope-collapsed canonical tools
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListDataPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_dataplanes", "data plane",
		"List data planes. With scope=\"namespace\" (default) lists a namespace's data planes (requires "+
			"namespace_name); with scope=\"cluster\" lists cluster-scoped data planes shared by platform admins. "+
			"Data planes are the Kubernetes clusters or cluster regions where component workloads execute. "+
			"Supports pagination via limit and cursor.",
		authzcore.ActionViewDataPlane, authzcore.ActionViewClusterDataPlane,
		scopedListHandlers{
			namespace: t.PEToolset.ListDataPlanes,
			cluster:   t.PEToolset.ListClusterDataPlanes,
		})
}

func (t *Toolsets) RegisterGetDataPlane(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_dataplane", "data plane",
		"Get detailed information about a data plane including cluster details, capacity, health status, "+
			"associated environments, and network configuration. Use scope=\"cluster\" for a cluster-scoped data plane.",
		authzcore.ActionViewDataPlane, authzcore.ActionViewClusterDataPlane,
		"name", "Data plane name. Use list_dataplanes to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetDataPlane,
			cluster:   t.PEToolset.GetClusterDataPlane,
		})
}

func (t *Toolsets) RegisterListWorkflowPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_workflowplanes", "workflow plane",
		"List workflow planes. With scope=\"namespace\" (default) lists a namespace's workflow planes (requires "+
			"namespace_name); with scope=\"cluster\" lists cluster-scoped workflow planes shared by platform admins. "+
			"Workflow planes handle continuous integration and container image building. "+
			"Supports pagination via limit and cursor.",
		authzcore.ActionViewWorkflowPlane, authzcore.ActionViewClusterWorkflowPlane,
		scopedListHandlers{
			namespace: t.PEToolset.ListWorkflowPlanes,
			cluster:   t.PEToolset.ListClusterWorkflowPlanes,
		})
}

func (t *Toolsets) RegisterGetWorkflowPlane(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_workflowplane", "workflow plane",
		"Get detailed information about a workflow plane including cluster details, health status, and agent "+
			"connection state. Use scope=\"cluster\" for a cluster-scoped workflow plane.",
		authzcore.ActionViewWorkflowPlane, authzcore.ActionViewClusterWorkflowPlane,
		"name", "Workflow plane name. Use list_workflowplanes to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetWorkflowPlane,
			cluster:   t.PEToolset.GetClusterWorkflowPlane,
		})
}

func (t *Toolsets) RegisterListObservabilityPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_observability_planes", "observability plane",
		"List observability planes. With scope=\"namespace\" (default) lists a namespace's observability planes "+
			"(requires namespace_name); with scope=\"cluster\" lists cluster-scoped observability planes shared by "+
			"platform admins. Observability planes provide monitoring, logging, tracing, and metrics collection for "+
			"deployed components. Supports pagination via limit and cursor.",
		authzcore.ActionViewObservabilityPlane, authzcore.ActionViewClusterObservabilityPlane,
		scopedListHandlers{
			namespace: t.PEToolset.ListObservabilityPlanes,
			cluster:   t.PEToolset.ListClusterObservabilityPlanes,
		})
}

func (t *Toolsets) RegisterGetObservabilityPlane(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_observability_plane", "observability plane",
		"Get detailed information about an observability plane including observer URL, health status, and agent "+
			"connection state. Use scope=\"cluster\" for a cluster-scoped observability plane.",
		authzcore.ActionViewObservabilityPlane, authzcore.ActionViewClusterObservabilityPlane,
		"name", "Observability plane name. Use list_observability_planes to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetObservabilityPlane,
			cluster:   t.PEToolset.GetClusterObservabilityPlane,
		})
}

// ---------------------------------------------------------------------------
// Deprecated cluster-prefixed aliases — component types
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterPEListClusterComponentTypes(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterListTool(s, perms, "list_cluster_component_types", "list_component_types",
		"Lists platform-wide cluster-scoped component types. Supports pagination via limit and cursor.",
		authzcore.ActionViewClusterComponentType, t.PEToolset.ListClusterComponentTypes)
}

func (t *Toolsets) RegisterListClusterComponentTypes(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterListTool(s, perms, "list_cluster_component_types", "list_component_types",
		"Lists platform-wide cluster-scoped component types. Supports pagination via limit and cursor.",
		authzcore.ActionViewClusterComponentType, t.ComponentToolset.ListClusterComponentTypes)
}

func (t *Toolsets) RegisterPEGetClusterComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_component_type", "get_component_type",
		"Gets the full definition of a platform-wide cluster-scoped component type.",
		authzcore.ActionViewClusterComponentType,
		"name", "Cluster component type name. Use list_cluster_component_types to discover valid names",
		t.PEToolset.GetClusterComponentType)
}

func (t *Toolsets) RegisterGetClusterComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_component_type", "get_component_type",
		"Gets the full definition of a platform-wide cluster-scoped component type.",
		authzcore.ActionViewClusterComponentType,
		"name", "Cluster component type name. Use list_cluster_component_types to discover valid names",
		t.ComponentToolset.GetClusterComponentType)
}

func (t *Toolsets) RegisterPEGetClusterComponentTypeSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_component_type_schema", "get_component_type_schema",
		"Gets the JSON schema for a platform-wide cluster-scoped component type.",
		authzcore.ActionViewClusterComponentType,
		"name", "Cluster component type name. Use list_cluster_component_types to discover valid names",
		t.PEToolset.GetClusterComponentTypeSchema)
}

func (t *Toolsets) RegisterGetClusterComponentTypeSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_component_type_schema", "get_component_type_schema",
		"Gets the JSON schema for a platform-wide cluster-scoped component type.",
		authzcore.ActionViewClusterComponentType,
		"name", "Cluster component type name. Use list_cluster_component_types to discover valid names",
		t.ComponentToolset.GetClusterComponentTypeSchema)
}

func (t *Toolsets) RegisterGetClusterComponentTypeCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterSchemaTool(s, perms,
		"get_cluster_component_type_creation_schema", "get_component_type_creation_schema",
		"Returns the spec schema for creating a platform-wide cluster-scoped component type.",
		authzcore.ActionCreateClusterComponentType,
		func() (any, error) { return ClusterComponentTypeCreationSchema() })
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterCreateClusterComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterWriteTool(s, perms, "create_cluster_component_type", "create_component_type",
		"Creates a platform-wide cluster-scoped component type available to all namespaces.",
		authzcore.ActionCreateClusterComponentType,
		"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)",
		"Use get_cluster_component_type_creation_schema to check the schema",
		func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
			spec, err := buildSpec[gen.ClusterComponentTypeSpec](specRaw)
			if err != nil {
				return nil, err
			}
			return t.PEToolset.CreateClusterComponentType(ctx, &gen.CreateClusterComponentTypeJSONRequestBody{
				Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
			})
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterUpdateClusterComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterWriteTool(s, perms, "update_cluster_component_type", "update_component_type",
		"Updates a platform-wide cluster-scoped component type (full replacement).",
		authzcore.ActionUpdateClusterComponentType,
		"Name of the cluster component type to update. Use list_cluster_component_types to discover valid names",
		"Full cluster component type spec to replace the existing one. "+
			"Use get_cluster_component_type to retrieve the current spec first.",
		func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
			spec, err := buildSpec[gen.ClusterComponentTypeSpec](specRaw)
			if err != nil {
				return nil, err
			}
			return t.PEToolset.UpdateClusterComponentType(ctx, &gen.UpdateClusterComponentTypeJSONRequestBody{
				Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
			})
		})
}

func (t *Toolsets) RegisterDeleteClusterComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "delete_cluster_component_type", "delete_component_type",
		"Deletes a platform-wide cluster-scoped component type.",
		authzcore.ActionDeleteClusterComponentType,
		"name", "Name of the cluster component type to delete. Use list_cluster_component_types to discover valid names",
		t.PEToolset.DeleteClusterComponentType)
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

// ---------------------------------------------------------------------------
// Deprecated cluster-prefixed aliases — workflows
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListClusterWorkflows(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterListTool(s, perms, "list_cluster_workflows", "list_workflows",
		"Lists platform-wide cluster-scoped workflows. Supports pagination via limit and cursor.",
		authzcore.ActionViewClusterWorkflow, t.BuildToolset.ListClusterWorkflows)
}

func (t *Toolsets) RegisterPEListClusterWorkflows(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterListTool(s, perms, "list_cluster_workflows", "list_workflows",
		"Lists platform-wide cluster-scoped workflows. Supports pagination via limit and cursor.",
		authzcore.ActionViewClusterWorkflow, t.PEToolset.ListClusterWorkflows)
}

func (t *Toolsets) RegisterGetClusterWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_workflow", "get_workflow",
		"Gets the full definition of a platform-wide cluster-scoped workflow.",
		authzcore.ActionViewClusterWorkflow,
		"name", "Cluster workflow name. Use list_cluster_workflows to discover valid names",
		t.BuildToolset.GetClusterWorkflow)
}

func (t *Toolsets) RegisterPEGetClusterWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_workflow", "get_workflow",
		"Gets the full definition of a platform-wide cluster-scoped workflow.",
		authzcore.ActionViewClusterWorkflow,
		"name", "Cluster workflow name. Use list_cluster_workflows to discover valid names",
		t.PEToolset.GetClusterWorkflow)
}

func (t *Toolsets) RegisterGetClusterWorkflowSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_workflow_schema", "get_workflow_schema",
		"Gets the schema for a platform-wide cluster-scoped workflow.",
		authzcore.ActionViewClusterWorkflow,
		"name", "Cluster workflow name. Use list_cluster_workflows to discover valid names",
		t.BuildToolset.GetClusterWorkflowSchema)
}

func (t *Toolsets) RegisterPEGetClusterWorkflowSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_workflow_schema", "get_workflow_schema",
		"Gets the schema for a platform-wide cluster-scoped workflow.",
		authzcore.ActionViewClusterWorkflow,
		"name", "Cluster workflow name. Use list_cluster_workflows to discover valid names",
		t.PEToolset.GetClusterWorkflowSchema)
}

func (t *Toolsets) RegisterGetClusterWorkflowCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterSchemaTool(s, perms,
		"get_cluster_workflow_creation_schema", "get_workflow_creation_schema",
		"Returns the spec schema for creating a platform-wide cluster-scoped workflow.",
		authzcore.ActionCreateClusterWorkflow,
		func() (any, error) { return ClusterWorkflowCreationSchema() })
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterCreateClusterWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterWriteTool(s, perms, "create_cluster_workflow", "create_workflow",
		"Creates a platform-wide cluster-scoped workflow available to all namespaces.",
		authzcore.ActionCreateClusterWorkflow,
		"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)",
		"Cluster workflow specification. Required field: runTemplate (Argo Workflow template definition). "+
			"Use get_cluster_workflow_schema on an existing workflow to see the full structure.",
		func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
			spec, err := buildSpec[gen.ClusterWorkflowSpec](specRaw)
			if err != nil {
				return nil, err
			}
			return t.PEToolset.CreateClusterWorkflow(ctx, &gen.CreateClusterWorkflowJSONRequestBody{
				Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
			})
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterUpdateClusterWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterWriteTool(s, perms, "update_cluster_workflow", "update_workflow",
		"Updates a platform-wide cluster-scoped workflow (full replacement). "+
			"Use get_cluster_workflow to retrieve the current definition first.",
		authzcore.ActionUpdateClusterWorkflow,
		"Name of the cluster workflow to update. Use list_cluster_workflows to discover valid names",
		"Full cluster workflow spec to replace the existing one. "+
			"Use get_cluster_workflow to retrieve the current spec first.",
		func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
			spec, err := buildSpec[gen.ClusterWorkflowSpec](specRaw)
			if err != nil {
				return nil, err
			}
			return t.PEToolset.UpdateClusterWorkflow(ctx, &gen.UpdateClusterWorkflowJSONRequestBody{
				Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
			})
		})
}

func (t *Toolsets) RegisterDeleteClusterWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "delete_cluster_workflow", "delete_workflow",
		"Deletes a platform-wide cluster-scoped workflow.",
		authzcore.ActionDeleteClusterWorkflow,
		"name", "Name of the cluster workflow to delete. Use list_cluster_workflows to discover valid names",
		t.PEToolset.DeleteClusterWorkflow)
}

// ---------------------------------------------------------------------------
// Deprecated cluster-prefixed aliases — plane resources
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListClusterDataPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterListTool(s, perms, "list_cluster_dataplanes", "list_dataplanes",
		"Lists cluster-scoped data planes shared by platform admins. Supports pagination via limit and cursor.",
		authzcore.ActionViewClusterDataPlane, t.PEToolset.ListClusterDataPlanes)
}

func (t *Toolsets) RegisterGetClusterDataPlane(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_dataplane", "get_dataplane",
		"Gets detailed information about a cluster-scoped data plane including cluster details, "+
			"capacity, health status, and network configuration.",
		authzcore.ActionViewClusterDataPlane,
		"name", "Cluster data plane name. Use list_cluster_dataplanes to discover valid names",
		t.PEToolset.GetClusterDataPlane)
}

func (t *Toolsets) RegisterListClusterWorkflowPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterListTool(s, perms, "list_cluster_workflowplanes", "list_workflowplanes",
		"Lists cluster-scoped workflow planes shared by platform admins. "+
			"Supports pagination via limit and cursor.",
		authzcore.ActionViewClusterWorkflowPlane, t.PEToolset.ListClusterWorkflowPlanes)
}

func (t *Toolsets) RegisterGetClusterWorkflowPlane(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_workflowplane", "get_workflowplane",
		"Gets detailed information about a cluster-scoped workflow plane including cluster details, "+
			"health status, and agent connection state.",
		authzcore.ActionViewClusterWorkflowPlane,
		"name", "Cluster workflow plane name. Use list_cluster_workflowplanes to discover valid names",
		t.PEToolset.GetClusterWorkflowPlane)
}

func (t *Toolsets) RegisterListClusterObservabilityPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterListTool(s, perms, "list_cluster_observability_planes", "list_observability_planes",
		"Lists cluster-scoped observability planes shared by platform admins. "+
			"Supports pagination via limit and cursor.",
		authzcore.ActionViewClusterObservabilityPlane, t.PEToolset.ListClusterObservabilityPlanes)
}

func (t *Toolsets) RegisterGetClusterObservabilityPlane(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_observability_plane", "get_observability_plane",
		"Gets detailed information about a cluster-scoped observability plane including observer URL, "+
			"health status, and agent connection state.",
		authzcore.ActionViewClusterObservabilityPlane,
		"name", "Cluster observability plane name. Use list_cluster_observability_planes to discover valid names",
		t.PEToolset.GetClusterObservabilityPlane)
}
