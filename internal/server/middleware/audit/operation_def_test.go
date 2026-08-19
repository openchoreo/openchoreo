// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import "testing"

func TestOperations(t *testing.T) {
	defs := []OperationDef{
		{
			ID: testProjectOpID, Action: "create_project", ResourceType: "project", Category: CategoryManagement,
			MCPToolName: "create_project", MCPResourceArg: "name",
		},
		{ID: "CreateDataPlane", Action: "create_dataplane", ResourceType: "dataplane", Category: CategoryManagement},
	}

	ops := Operations(defs)
	if len(ops) != len(defs) {
		t.Fatalf("len(Operations(defs)) = %d, want %d", len(ops), len(defs))
	}

	want := Operation{ID: testProjectOpID, Action: "create_project", ResourceType: "project", Category: CategoryManagement}
	if ops[0] != want {
		t.Errorf("Operations(defs)[0] = %+v, want %+v (MCP fields must not leak into Operation)", ops[0], want)
	}
}

func TestMCPBindings_DerivedFromDefs(t *testing.T) {
	defs := []OperationDef{
		{
			ID: testProjectOpID, Action: "create_project", ResourceType: "project", Category: CategoryManagement,
			MCPToolName: "create_project", MCPResourceArg: "name",
		},
		{
			// No MCPToolName: DataPlane has no MCP tool, so this must be
			// absent from the result rather than appearing under an empty
			// tool-name key.
			ID: "CreateDataPlane", Action: "create_dataplane", ResourceType: "dataplane", Category: CategoryManagement,
		},
	}

	bindings := MCPBindings(defs)
	if len(bindings) != 1 {
		t.Fatalf("len(MCPBindings(defs)) = %d, want 1 (only defs with MCPToolName set)", len(bindings))
	}

	b, ok := bindings["create_project"]
	if !ok {
		t.Fatal(`MCPBindings(defs)["create_project"] missing`)
	}
	if b.ResourceArg != "name" {
		t.Errorf("ResourceArg = %q, want %q", b.ResourceArg, "name")
	}
	if b.Operation == nil || b.Operation.ID != testProjectOpID {
		t.Errorf("Operation = %+v, want ID CreateProject", b.Operation)
	}

	if _, ok := bindings[""]; ok {
		t.Error(`MCPBindings(defs) has a spurious entry under the empty-string key for the def with no MCPToolName`)
	}
}
