// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import "fmt"

// MCPBinding declares how one audited Operation maps onto an MCP tool. It
// carries the fully resolved Operation rather than an ID to look up
// elsewhere, so a binding can't drift apart from its Operation. Lives in core
// rather than an MCP-specific package since it has no MCP SDK dependency —
// any service exposing MCP tools alongside a REST API can reuse it.
//
// ResourceArg is the JSON argument name — from the tool's raw call arguments
// — that carries the resource's identifying name. Declared per binding, never
// derived: MCP argument names are ad hoc (e.g. create_project uses "name",
// update_project uses "project_name" for the same resource).
type MCPBinding struct {
	Operation   *Operation
	ResourceArg string
}

// MCPBindingKey identifies one entry in the MCP binding table: a tool name,
// plus an opaque scope discriminator for a tool that routes to different
// operations based on a call argument (a "scope-collapsed" tool — e.g. one
// MCP tool fanning out to both a namespace-scoped and a cluster-scoped REST
// operation). Scope is the empty string for the common case of a tool bound
// to exactly one operation.
//
// Scope is deliberately opaque here: core has no MCP-SDK or pkg/mcp/tools
// dependency, so it doesn't know what a valid scope value is or what it
// means — only that two OperationDefs sharing a tool name must be
// distinguished by *something*. The adapter (pkg/mcp/mcpaudit) is the layer
// that knows the real scope values and how to resolve one from a call.
type MCPBindingKey struct {
	ToolName string
	Scope    string
}

// MCPAlias declares an additional MCP tool name that binds to the same
// operation as an existing OperationDef, referenced by ID rather than by a
// duplicated OperationDef row.
//
// OperationDef.MCPToolName holds exactly one tool name per operation, so it
// can't express two or more tool names reaching the same operation (e.g. a
// deprecated alias of a canonical tool). Duplicating the OperationDef row
// for that would give BuildPatternMap two operations resolving to the same
// REST pattern — a false collision, since both rows describe one real
// route. MCPAlias avoids that by attaching only to the binding table, never
// touching Operations().
type MCPAlias struct {
	// OperationID must match an existing OperationDef.ID.
	OperationID string
	// ToolName is the additional MCP tool name bound to that operation.
	ToolName string
	// Scope discriminates a scope-collapsed alias, mirroring
	// OperationDef.MCPScope. Empty for the common case.
	Scope string
	// ResourceArg is the JSON argument name carrying the resource's
	// identifying name in this alias tool's call arguments — declared
	// separately from the canonical binding's, since an alias tool can use a
	// different argument shape (e.g. trigger_workflow_run vs
	// create_workflow_run).
	ResourceArg string
}

// MergeMCPAliases adds one binding per alias into bindings, resolving each
// alias's Operation by looking up its OperationID in defs. Mutates bindings
// in place so callers can build the canonical table with MCPBindings and
// layer aliases on top with one further call.
//
// Returns an error if an alias references an operation ID absent from defs
// (an alias table typo, or an operation renamed/removed without updating its
// aliases), if an alias collides with an existing (ToolName, Scope) key — the
// same silent-overwrite failure mode MCPBindings itself guards against — or if
// an alias would mix an unscoped and a scoped binding for one tool name (see
// scopeTracker.checkAndRecord), checked against bindings' existing entries as
// well as against other aliases in this same call.
func MergeMCPAliases(defs []OperationDef, bindings map[MCPBindingKey]MCPBinding, aliases []MCPAlias) error {
	byID := make(map[string]*Operation, len(defs))
	for i := range defs {
		d := &defs[i]
		byID[d.ID] = &Operation{
			ID: d.ID, Action: d.Action, ResourceType: d.ResourceType, Category: d.Category,
			RESTResourceParam: d.RESTResourceParam, NotInOpenAPISpec: d.NotInOpenAPISpec,
		}
	}

	tracker := newScopeTrackerFromBindings(bindings)
	for _, a := range aliases {
		op, ok := byID[a.OperationID]
		if !ok {
			return fmt.Errorf("audit: MCP alias %q (scope %q) references unknown operation %q",
				a.ToolName, a.Scope, a.OperationID)
		}
		key := MCPBindingKey{ToolName: a.ToolName, Scope: a.Scope}
		if existing, collides := bindings[key]; collides {
			return fmt.Errorf("audit: MCP alias %q (scope %q) collides with an existing binding to operation %q",
				a.ToolName, a.Scope, existing.Operation.ID)
		}
		if err := tracker.checkAndRecord(a.ToolName, a.Scope); err != nil {
			return err
		}
		bindings[key] = MCPBinding{Operation: op, ResourceArg: a.ResourceArg}
	}
	return nil
}
