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

	bindings, err := MCPBindings(defs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("len(MCPBindings(defs)) = %d, want 1 (only defs with MCPToolName set)", len(bindings))
	}

	key := MCPBindingKey{ToolName: "create_project"}
	b, ok := bindings[key]
	if !ok {
		t.Fatal(`MCPBindings(defs)[{ToolName: "create_project"}] missing`)
	}
	if b.ResourceArg != "name" {
		t.Errorf("ResourceArg = %q, want %q", b.ResourceArg, "name")
	}
	if b.Operation == nil || b.Operation.ID != testProjectOpID {
		t.Errorf("Operation = %+v, want ID CreateProject", b.Operation)
	}

	if _, ok := bindings[MCPBindingKey{}]; ok {
		t.Error(`MCPBindings(defs) has a spurious entry under the empty-key for the def with no MCPToolName`)
	}
}

// TestMCPBindings_ScopeCollapsedFanOut is the 0.10e proof: two OperationDefs
// sharing one MCPToolName but distinguished by MCPScope (a scope-collapsed
// tool, e.g. create_component_type routing to either a namespace-scoped or a
// cluster-scoped REST operation) must resolve to two distinct entries, not
// one silently overwriting the other.
func TestMCPBindings_ScopeCollapsedFanOut(t *testing.T) {
	defs := []OperationDef{
		{
			ID: "CreateComponentType", Action: "create_component_type", ResourceType: "componenttype",
			Category: CategoryManagement, MCPToolName: "create_component_type", MCPScope: "namespace", MCPResourceArg: "name",
		},
		{
			ID: "CreateClusterComponentType", Action: "create_cluster_component_type", ResourceType: "clustercomponenttype",
			Category: CategoryManagement, MCPToolName: "create_component_type", MCPScope: "cluster", MCPResourceArg: "name",
		},
	}

	bindings, err := MCPBindings(defs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bindings) != 2 {
		t.Fatalf("len(MCPBindings(defs)) = %d, want 2 (one per scope)", len(bindings))
	}

	ns, ok := bindings[MCPBindingKey{ToolName: "create_component_type", Scope: "namespace"}]
	if !ok || ns.Operation == nil || ns.Operation.ID != "CreateComponentType" {
		t.Errorf("namespace-scope binding = %+v, want Operation.ID CreateComponentType", ns)
	}
	cluster, ok := bindings[MCPBindingKey{ToolName: "create_component_type", Scope: "cluster"}]
	if !ok || cluster.Operation == nil || cluster.Operation.ID != "CreateClusterComponentType" {
		t.Errorf("cluster-scope binding = %+v, want Operation.ID CreateClusterComponentType", cluster)
	}
}

// TestMCPBindings_DuplicateKeyErrors guards the failure mode a bare map
// write used to allow silently: two defs resolving to the same
// (MCPToolName, MCPScope) key, where the second would overwrite the first
// with no signal — misattributing every call routed to the first def's tool
// to the second def's action/category/resource type instead.
func TestMCPBindings_DuplicateKeyErrors(t *testing.T) {
	defs := []OperationDef{
		{ID: "CreateComponentType", Action: "create_component_type", Category: CategoryManagement, MCPToolName: "create_component_type"},
		{ID: "CreateClusterComponentType", Action: "create_cluster_component_type", Category: CategoryManagement, MCPToolName: "create_component_type"},
	}

	_, err := MCPBindings(defs)
	if err == nil {
		t.Fatal("expected an error for two defs colliding on the same (MCPToolName, MCPScope) key, got nil")
	}
}

// TestMCPBindings_RejectsMixedScopedAndUnscoped guards a defect that doesn't
// collide on (MCPToolName, MCPScope) — {tool, ""} and {tool, "cluster"} are
// distinct map entries — but is still wrong: the adapter's resolveBinding
// always tries the unscoped key first, so the scoped binding would be
// silently unreachable and every call to the tool would resolve to the
// unscoped operation regardless of its actual scope argument.
func TestMCPBindings_RejectsMixedScopedAndUnscoped(t *testing.T) {
	defs := []OperationDef{
		{ID: "CreateComponentType", Action: "create_component_type", Category: CategoryManagement, MCPToolName: "create_component_type"},
		{
			ID: "CreateClusterComponentType", Action: "create_cluster_component_type", Category: CategoryManagement,
			MCPToolName: "create_component_type", MCPScope: "cluster",
		},
	}

	_, err := MCPBindings(defs)
	if err == nil {
		t.Fatal("expected an error for a tool mixing an unscoped and a scoped binding, got nil")
	}
}

// TestMCPBindings_RejectsMixedScopedAndUnscoped_ScopedFirst is the same
// defect with the unscoped def appearing second — the check must catch it
// regardless of which def in the slice declares the empty scope.
func TestMCPBindings_RejectsMixedScopedAndUnscoped_ScopedFirst(t *testing.T) {
	defs := []OperationDef{
		{
			ID: "CreateClusterComponentType", Action: "create_cluster_component_type", Category: CategoryManagement,
			MCPToolName: "create_component_type", MCPScope: "cluster",
		},
		{ID: "CreateComponentType", Action: "create_component_type", Category: CategoryManagement, MCPToolName: "create_component_type"},
	}

	_, err := MCPBindings(defs)
	if err == nil {
		t.Fatal("expected an error for a tool mixing a scoped and an unscoped binding, got nil")
	}
}
