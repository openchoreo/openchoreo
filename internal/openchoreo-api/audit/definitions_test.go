// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// TestGetOperations guards the shape every Operation must have, and the
// overall category split derived from generatedOperationDefs plus the two
// hand-declared non-spec operations (Exec, Wirelogs) — see tools/auditgen's
// TestBuildDefinitions_AgainstLiveSpec for the generator's own regression
// guard on the spec-derived counts alone.
func TestGetOperations(t *testing.T) {
	ops := GetOperations()

	const want = 114
	if len(ops) != want {
		t.Fatalf("len(GetOperations()) = %d, want %d", len(ops), want)
	}

	seen := make(map[string]bool, len(ops))
	var mgmt, authz, notInSpec int
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
		switch op.Category {
		case audit.CategoryManagement:
			mgmt++
		case audit.CategoryAuthorization:
			authz++
		default:
			t.Errorf("operation %q has unrecognized category %q", op.ID, op.Category)
		}
		if op.NotInOpenAPISpec {
			notInSpec++
		}
		if seen[op.ID] {
			t.Errorf("duplicate operation ID %q", op.ID)
		}
		seen[op.ID] = true
	}

	if mgmt != 102 || authz != 12 {
		t.Errorf("category split = %d management / %d authorization, want 102/12", mgmt, authz)
	}
	if notInSpec != 2 {
		t.Errorf("NotInOpenAPISpec count = %d, want 2 (Exec, Wirelogs)", notInSpec)
	}
}

// TestMCPBindings is a direct, readable regression test over key shapes in
// the full MCP tool-to-operation binding table (69 tool names, 90 (tool,
// scope) keys: 38 plain + 21 scope-collapsed tools x 2 operations + 10
// fan-in aliases). The exhaustive
// structural checks (every tool bound or exempted, no unresolvable
// reference) live in mcphandlers' TestAuditCoverage, which cross-references
// the live MCP tool registry; this test instead pins specific, easy-to-get-
// wrong cases so a regression here fails with a precise diff.
func TestMCPBindings(t *testing.T) {
	bindings, err := MCPBindings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	const wantTotalKeys = 90
	if len(bindings) != wantTotalKeys {
		t.Fatalf("len(MCPBindings()) = %d, want %d (got tools: %v)", len(bindings), wantTotalKeys, toolNames(bindings))
	}

	// Plain (unscoped) bindings.
	plain := map[string]struct {
		operationID, resourceArg string
	}{
		"create_project":     {"CreateProject", "name"},
		"update_project":     {"UpdateProject", "project_name"},
		"delete_project":     {"DeleteProject", "project_name"},
		"create_environment": {"CreateEnvironment", "name"},
		"update_environment": {"UpdateEnvironment", "name"},
		"delete_environment": {"DeleteEnvironment", "name"},
		// create_component_release's target is GenerateRelease, not a
		// "CreateComponentRelease" operation — both it and REST's
		// GenerateRelease handler call ComponentService.GenerateRelease.
		"create_component_release": {"GenerateRelease", "release_name"},
		// No resource-name argument carries the *created* resource's own
		// identity for these three — see mcpEnrichment's doc comment.
		"create_workload":        {"CreateWorkload", ""},
		"create_release_binding": {"CreateReleaseBinding", ""},
		"create_workflow_run":    {"CreateWorkflowRun", ""},
	}
	for tool, want := range plain {
		key := audit.MCPBindingKey{ToolName: tool}
		b, ok := bindings[key]
		if !ok {
			t.Errorf("MCPBindings() missing entry for tool %q", tool)
			continue
		}
		if b.Operation == nil || b.Operation.ID != want.operationID {
			t.Errorf("MCPBindings()[%q].Operation = %+v, want ID %q", tool, b.Operation, want.operationID)
		}
		if b.ResourceArg != want.resourceArg {
			t.Errorf("MCPBindings()[%q].ResourceArg = %q, want %q", tool, b.ResourceArg, want.resourceArg)
		}
	}

	// Scope-collapsed: one tool, two operations by scope.
	ns, ok := bindings[audit.MCPBindingKey{ToolName: "create_authz_role", Scope: "namespace"}]
	if !ok || ns.Operation == nil || ns.Operation.ID != "CreateNamespaceRole" {
		t.Errorf(`bindings[{"create_authz_role","namespace"}] = %+v, want Operation.ID CreateNamespaceRole`, ns)
	}
	cluster, ok := bindings[audit.MCPBindingKey{ToolName: "create_authz_role", Scope: "cluster"}]
	if !ok || cluster.Operation == nil || cluster.Operation.ID != "CreateClusterRole" {
		t.Errorf(`bindings[{"create_authz_role","cluster"}] = %+v, want Operation.ID CreateClusterRole`, cluster)
	}

	// Fan-in: trigger_workflow_run is a second, unscoped entry point onto the
	// same operation as create_workflow_run.
	trigger, ok := bindings[audit.MCPBindingKey{ToolName: "trigger_workflow_run"}]
	if !ok || trigger.Operation == nil || trigger.Operation.ID != "CreateWorkflowRun" {
		t.Errorf(`bindings[{"trigger_workflow_run",""}] = %+v, want Operation.ID CreateWorkflowRun`, trigger)
	}

	// Deprecated cluster-prefixed alias: unscoped tool name, cluster operation.
	deprecatedAlias, ok := bindings[audit.MCPBindingKey{ToolName: "create_cluster_component_type"}]
	if !ok || deprecatedAlias.Operation == nil || deprecatedAlias.Operation.ID != "CreateClusterComponentType" {
		t.Errorf(`bindings[{"create_cluster_component_type",""}] = %+v, want Operation.ID CreateClusterComponentType`,
			deprecatedAlias)
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

// TestValidateEnrichmentKeys_RealTableIsClean directly asserts the real
// mcpEnrichment table validates against the real operation ID set — the
// init() in mcp_bindings.go already guards this at package load (a failure
// there panics the test binary before any test runs), but this pins the
// specific, non-panicking success case as a normal, readable test result.
func TestValidateEnrichmentKeys_RealTableIsClean(t *testing.T) {
	all := append(generatedOperationDefs(), nonSpecOperationDefs...)
	if err := validateEnrichmentKeys(all, mcpEnrichment); err != nil {
		t.Fatalf("validateEnrichmentKeys() error = %v, want nil for the real mcpEnrichment table", err)
	}
}

// TestValidateEnrichmentKeys_TypoErrors guards the actual bug this check
// exists for: a typo'd mcpEnrichment key doesn't collide with anything and
// isn't referenced by anything else, so nothing before this check would ever
// catch it — the intended operation just silently keeps its zero-value MCP
// fields, and the tool goes unbound with no error anywhere.
func TestValidateEnrichmentKeys_TypoErrors(t *testing.T) {
	defs := []audit.OperationDef{{ID: "CreateProject"}}
	enrichment := map[string]mcpEnrichmentEntry{
		"CreateProjct": {ToolName: "create_project", ResourceArg: "name"},
	}

	err := validateEnrichmentKeys(defs, enrichment)
	if err == nil {
		t.Fatal("validateEnrichmentKeys() = nil error, want an error for a key with no matching operation ID")
	}
	if !strings.Contains(err.Error(), "CreateProjct") {
		t.Errorf("validateEnrichmentKeys() error = %q, want it to name the bad key %q", err.Error(), "CreateProjct")
	}
}

func toolNames(bindings map[audit.MCPBindingKey]audit.MCPBinding) []string {
	names := make([]string, 0, len(bindings))
	for key := range bindings {
		names = append(names, key.ToolName)
	}
	return names
}
