// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import "fmt"

// OperationDef is one service's definition of an audited operation. A service
// declares a table of these as its single source of truth; Operations and
// MCPBindings derive each surface's view from it. Carries no service-specific
// imports, so it lives in core instead of being redeclared per service.
type OperationDef struct {
	// ID is the OpenAPI operationId, e.g. "CreateProject" — 1:1 with a REST
	// route. BuildPatternMap cross-references it against the live spec.
	ID string
	// Action is the semantic audit action, e.g. "create_project".
	Action string
	// ResourceType is derived from the operation, e.g. "projects".
	ResourceType string
	// Category is stamped from the operation's resource kind.
	Category Category

	// MCPToolName is the MCP tool bound to this operation, if any. Empty
	// means not exposed via MCP.
	MCPToolName string
	// MCPScope discriminates two OperationDefs that share an MCPToolName —
	// a scope-collapsed tool that routes to different operations based on a
	// call argument (e.g. one tool fanning out to a namespace-scoped and a
	// cluster-scoped REST operation). Leave empty for the common case of a
	// tool bound to exactly one operation. Only meaningful when MCPToolName
	// is set. An opaque string here — see MCPBindingKey's doc comment for why.
	MCPScope string
	// MCPResourceArg is the JSON argument name in the tool's call arguments
	// that carries the resource's identifying name. Only meaningful when
	// MCPToolName is set — declared per operation since MCP argument names
	// don't mechanically align with REST path parameters.
	MCPResourceArg string
	// RESTResourceParam is the REST path parameter carrying the resource's
	// identifying name, e.g. "projectName". Empty for operations with no
	// resource name in the path (e.g. Create routes).
	RESTResourceParam string
}

// Operations returns every audited Operation in the surface-neutral shape the
// REST resolver consumes — a view over defs with the MCP fields dropped.
func Operations(defs []OperationDef) []Operation {
	ops := make([]Operation, len(defs))
	for i, d := range defs {
		ops[i] = Operation{
			ID: d.ID, Action: d.Action, ResourceType: d.ResourceType, Category: d.Category,
			RESTResourceParam: d.RESTResourceParam,
		}
	}
	return ops
}

// MCPBindings derives the MCP tool-to-operation binding table, keyed by
// (tool name, scope), from the same defs table Operations reads. A def with
// no MCPToolName is simply absent from the result.
//
// Returns an error if two defs resolve to the same (MCPToolName, MCPScope)
// key — a scope-collapsed tool wired to two operations that don't
// distinguish themselves by scope, or a plain copy-paste duplicate. Building
// this with a bare map write would let the second def silently win, and the
// adapter would then mis-attribute every call routed to the first: the wrong
// action, category and resource type recorded for a real mutation. Fail
// loudly at construction time instead — the same principle BuildPatternMap
// already applies to a REST pattern collision.
//
// Also rejects one MCPToolName mixing an unscoped def (MCPScope == "") with
// a scoped one: that combination doesn't collide on the key above — {tool,
// ""} and {tool, "cluster"} are distinct map entries — but the adapter's
// resolveBinding always tries the unscoped key first, so the scoped binding
// would be silently unreachable and every call to that tool would resolve
// to the unscoped operation regardless of its actual scope argument.
func MCPBindings(defs []OperationDef) (map[MCPBindingKey]MCPBinding, error) {
	bindings := make(map[MCPBindingKey]MCPBinding)
	scopesSeen := make(map[string]map[string]bool) // tool name -> set of scopes already bound
	for _, d := range defs {
		if d.MCPToolName == "" {
			continue
		}
		seen := scopesSeen[d.MCPToolName]
		if d.MCPScope == "" && len(seen) != 0 {
			return nil, fmt.Errorf("audit: MCP tool %q cannot mix an unscoped binding with a scoped one", d.MCPToolName)
		}
		if d.MCPScope != "" && seen[""] {
			return nil, fmt.Errorf("audit: MCP tool %q cannot mix an unscoped binding with a scoped one", d.MCPToolName)
		}
		if seen == nil {
			seen = make(map[string]bool)
			scopesSeen[d.MCPToolName] = seen
		}
		seen[d.MCPScope] = true

		key := MCPBindingKey{ToolName: d.MCPToolName, Scope: d.MCPScope}
		if existing, collides := bindings[key]; collides {
			return nil, fmt.Errorf("audit: operations %q and %q both bind MCP tool %q with scope %q",
				existing.Operation.ID, d.ID, d.MCPToolName, d.MCPScope)
		}
		bindings[key] = MCPBinding{
			Operation: &Operation{
				ID: d.ID, Action: d.Action, ResourceType: d.ResourceType, Category: d.Category,
				RESTResourceParam: d.RESTResourceParam,
			},
			ResourceArg: d.MCPResourceArg,
		}
	}
	return bindings, nil
}
