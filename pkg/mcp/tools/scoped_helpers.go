// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// Scope-collapsed tools
//
// A scope-collapsed tool serves both a namespace-scoped resource and its
// cluster-scoped counterpart (e.g. ComponentType / ClusterComponentType) from a
// single tool that selects behavior via a `scope` argument.
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
