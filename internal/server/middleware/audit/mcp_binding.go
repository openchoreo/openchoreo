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
