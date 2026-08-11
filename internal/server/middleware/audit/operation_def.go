// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

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
	// MCPResourceArg is the JSON argument name in the tool's call arguments
	// that carries the resource's identifying name. Only meaningful when
	// MCPToolName is set — declared per operation since MCP argument names
	// don't mechanically align with REST path parameters.
	MCPResourceArg string
}

// Operations returns every audited Operation in the surface-neutral shape the
// REST resolver consumes — a view over defs with the MCP fields dropped.
func Operations(defs []OperationDef) []Operation {
	ops := make([]Operation, len(defs))
	for i, d := range defs {
		ops[i] = Operation{ID: d.ID, Action: d.Action, ResourceType: d.ResourceType, Category: d.Category}
	}
	return ops
}

// MCPBindings derives the MCP tool-to-operation binding table, keyed by tool
// name, from the same defs table Operations reads. A def with no MCPToolName
// is simply absent from the result.
func MCPBindings(defs []OperationDef) map[string]MCPBinding {
	bindings := make(map[string]MCPBinding)
	for _, d := range defs {
		if d.MCPToolName == "" {
			continue
		}
		bindings[d.MCPToolName] = MCPBinding{
			Operation:   &Operation{ID: d.ID, Action: d.Action, ResourceType: d.ResourceType, Category: d.Category},
			ResourceArg: d.MCPResourceArg,
		}
	}
	return bindings
}
