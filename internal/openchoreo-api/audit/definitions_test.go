// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"testing"

	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

func TestGetOperations(t *testing.T) {
	ops := GetOperations()

	const want = 12
	if len(ops) != want {
		t.Fatalf("len(GetOperations()) = %d, want %d", len(ops), want)
	}

	seen := make(map[string]bool, len(ops))
	for _, op := range ops {
		if op.ID == "" {
			t.Errorf("operation has empty ID: %+v", op)
		}
		if op.Action == "" {
			t.Errorf("operation %q has empty Action", op.ID)
		}
		if op.ResourceType == "" {
			t.Errorf("operation %q has empty ResourceType", op.ID)
		}
		if op.Category != audit.CategoryManagement {
			t.Errorf("operation %q Category = %q, want %q", op.ID, op.Category, audit.CategoryManagement)
		}
		if seen[op.ID] {
			t.Errorf("duplicate operation ID %q", op.ID)
		}
		seen[op.ID] = true
	}
}

// TestMCPBindings guards the exact set of operations exposed via MCP: only
// Project and Environment create/update/delete have a tool today (DataPlane
// has no MCP tool at all, and Secret's MCP tools hit SecretReferenceService,
// a different resource) — see the doc comment on operationDefs.
func TestMCPBindings(t *testing.T) {
	bindings, err := MCPBindings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantToolResourceArg := map[string]string{
		"create_project":     "name",
		"update_project":     "project_name",
		"delete_project":     "project_name",
		"create_environment": "name",
		"update_environment": "name",
		"delete_environment": "name",
	}

	if len(bindings) != len(wantToolResourceArg) {
		t.Fatalf("len(MCPBindings()) = %d, want %d (got tools: %v)", len(bindings), len(wantToolResourceArg), toolNames(bindings))
	}

	// None of the P0 operations are scope-collapsed, so every key's Scope is
	// empty — see TestMCPBindings_ScopeCollapsedFanOut in the core package
	// for the scoped case.
	for tool, wantArg := range wantToolResourceArg {
		key := audit.MCPBindingKey{ToolName: tool}
		b, ok := bindings[key]
		if !ok {
			t.Errorf("MCPBindings() missing entry for tool %q", tool)
			continue
		}
		if b.Operation == nil {
			t.Errorf("MCPBindings()[%q].Operation is nil", tool)
			continue
		}
		if b.Operation.Action != tool {
			t.Errorf("MCPBindings()[%q].Operation.Action = %q, want %q (tool name and Action must match)",
				tool, b.Operation.Action, tool)
		}
		if b.ResourceArg != wantArg {
			t.Errorf("MCPBindings()[%q].ResourceArg = %q, want %q", tool, b.ResourceArg, wantArg)
		}
	}

	for _, unbound := range []string{
		"create_dataplane", "update_dataplane", "delete_dataplane",
		"create_secret", "update_secret", "delete_secret",
	} {
		if _, ok := bindings[audit.MCPBindingKey{ToolName: unbound}]; ok {
			t.Errorf("MCPBindings() unexpectedly has an entry for %q (DataPlane/Secret have no MCP tool)", unbound)
		}
	}
}

func toolNames(bindings map[audit.MCPBindingKey]audit.MCPBinding) []string {
	names := make([]string, 0, len(bindings))
	for key := range bindings {
		names = append(names, key.ToolName)
	}
	return names
}
