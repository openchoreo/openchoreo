// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

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
