// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	apiaudit "github.com/openchoreo/openchoreo/internal/openchoreo-api/audit"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// registerAllToolsets returns perms for every registered MCP tool, enabling
// every toolset regardless of what a given deployment's config turns on — see
// TestMCPBindings_MatchRegisteredTools's doc comment for why that's the right
// scope for a static coverage check.
func registerAllToolsets(t *testing.T) map[string]tools.ToolPermission {
	t.Helper()
	handler := NewMCPHandler(&handlerservices.Services{})
	toolsets := tools.NewToolsets(handler, tools.AllToolsetTypes())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	perms, _ := toolsets.Register(server)
	return perms
}

// toolInputSchemaProperties registers every toolset against a real MCP
// server, connects an in-memory client, and lists tools to read back each
// tool's actual input schema — the same schema a real MCP client sees. This
// is the only way to get at a registered tool's argument names: AddTool
// infers the schema from the handler's input struct via reflection, and
// nothing in pkg/mcp/tools exposes that inference result directly (see
// pkg/mcp/tools/registration_test.go, which uses the identical
// ListTools-over-in-memory-transport pattern to inspect registered tools).
func toolInputSchemaProperties(t *testing.T) map[string]map[string]bool {
	t.Helper()
	handler := NewMCPHandler(&handlerservices.Services{})
	toolsets := tools.NewToolsets(handler, tools.AllToolsetTypes())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	toolsets.Register(server)

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("failed to connect server: %v", err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("failed to connect client: %v", err)
	}
	defer clientSession.Close()

	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("failed to list tools: %v", err)
	}
	if toolsResult.NextCursor != "" {
		// DefaultPageSize (1000) comfortably fits all registered tools today, so
		// this is unreached. If it ever fires, a tool on a later page would
		// otherwise be silently missing from props and get reported by the
		// caller as "not registered" — a paging bug masquerading as a coverage
		// bug. Fail loudly here instead of leaving that to be diagnosed cold.
		t.Fatalf("ListTools returned a NextCursor — tools no longer fit in one page, "+
			"got %d tools", len(toolsResult.Tools))
	}

	props := make(map[string]map[string]bool, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			continue
		}
		names := make(map[string]bool)
		if p, ok := schema["properties"].(map[string]any); ok {
			for name := range p {
				names[name] = true
			}
		}
		props[tool.Name] = names
	}
	return props
}

// stateModifyingRESTOperationIDs returns every non-GET operationId in the
// live OpenAPI spec — the same universe tools/auditgen walks.
func stateModifyingRESTOperationIDs(t *testing.T) map[string]bool {
	t.Helper()
	swagger, err := gen.GetSwagger()
	if err != nil {
		t.Fatalf("failed to load swagger: %v", err)
	}
	ids := make(map[string]bool)
	for _, path := range swagger.Paths.InMatchingOrder() {
		item := swagger.Paths.Find(path)
		for method, op := range item.Operations() {
			if method == "GET" {
				continue
			}
			ids[op.OperationID] = true
		}
	}
	return ids
}

// TestAuditCoverage is a CI gate: every state-modifying operation on both
// surfaces must be audited or explicitly, reasoned-ly exempted, enforced at
// build time rather than left to drift silently.
func TestAuditCoverage(t *testing.T) {
	restOperationIDs := stateModifyingRESTOperationIDs(t)
	definedOps := apiaudit.GetOperations()
	definedByID := make(map[string]audit.Operation, len(definedOps))
	for _, op := range definedOps {
		definedByID[op.ID] = op
	}

	perms := registerAllToolsets(t)
	bindings, err := apiaudit.MCPBindings()
	if err != nil {
		t.Fatalf("apiaudit.MCPBindings() returned an error: %v", err)
	}
	boundToolNames := make(map[string]bool, len(bindings))
	for key := range bindings {
		boundToolNames[key.ToolName] = true
	}

	// Assertion 0: the full toolset count is pinned so a toolset silently
	// missing from registerAllToolsets — e.g. a new ToolsetType added to
	// tools.AllToolsetTypes without a corresponding handler method, or a
	// toolset accidentally left out of the enabled set — fails here with a
	// clear count mismatch instead of this test quietly covering fewer tools
	// than actually exist.
	t.Run("registers every known tool", func(t *testing.T) {
		const wantTools = 157
		if len(perms) != wantTools {
			t.Errorf("registerAllToolsets(t) registered %d tools, want %d", len(perms), wantTools)
		}
	})

	// Assertion 1: every state-modifying REST operation is defined or exempted.
	t.Run("every REST operation is defined or exempted", func(t *testing.T) {
		for id := range restOperationIDs {
			_, defined := definedByID[id]
			_, exempted := apiaudit.RESTExemptions[id]
			if !defined && !exempted {
				t.Errorf("operationId %q is neither audited (apiaudit.GetOperations) nor exempted "+
					"(apiaudit.RESTExemptions) — add one or the other", id)
			}
			if defined && exempted {
				t.Errorf("operationId %q is both audited and exempted — remove one", id)
			}
		}
	})

	// Assertion 2: every state-modifying (non-read-only) MCP tool is bound or exempted.
	t.Run("every state-modifying MCP tool is bound or exempted", func(t *testing.T) {
		for name, perm := range perms {
			if perm.IsReadOnly() {
				continue
			}
			_, bound := boundToolNames[name]
			_, exempted := apiaudit.MCPToolExemptions[name]
			if !bound && !exempted {
				t.Errorf("tool %q declares a non-view action %v but is neither bound "+
					"(apiaudit.MCPBindings) nor exempted (apiaudit.MCPToolExemptions)", name, perm.Actions())
			}
			if bound && exempted {
				t.Errorf("tool %q is both bound and exempted — remove one", name)
			}
		}
	})

	// Assertion 3: every MCP binding points at an operation that exists.
	// audit.MCPBindings / audit.MergeMCPAliases already enforce this at
	// construction (the err check above would have failed), and every
	// binding's Operation is either derived directly from operationDefs (the
	// embedded-field path) or resolved against it by ID (the alias path) — so
	// there is no way for a binding to reference a nonexistent operation and
	// reach this line. This re-asserts it defensively per-binding so a future
	// refactor that loosens that guarantee fails here, in a test that says
	// what it guards, rather than as a mysterious nil dereference downstream.
	t.Run("every MCP binding points at a real operation", func(t *testing.T) {
		for key, binding := range bindings {
			if binding.Operation == nil {
				t.Errorf("binding for tool %q (scope %q) has a nil Operation", key.ToolName, key.Scope)
				continue
			}
			if _, ok := definedByID[binding.Operation.ID]; !ok {
				t.Errorf("binding for tool %q (scope %q) references operation %q, "+
					"which is not in apiaudit.GetOperations()", key.ToolName, key.Scope, binding.Operation.ID)
			}
		}
	})

	// Assertion 4: every exemption references something real.
	t.Run("every exemption references something real", func(t *testing.T) {
		for id, reason := range apiaudit.RESTExemptions {
			if reason == "" {
				t.Errorf("REST exemption %q has an empty reason", id)
			}
			if !restOperationIDs[id] {
				t.Errorf("REST exemption %q does not name a real state-modifying operation in the live spec "+
					"(renamed or removed operationId?)", id)
			}
		}
		for name, reason := range apiaudit.MCPToolExemptions {
			if reason == "" {
				t.Errorf("MCP tool exemption %q has an empty reason", name)
			}
			perm, ok := perms[name]
			if !ok {
				t.Errorf("MCP tool exemption %q does not name a real registered tool "+
					"(renamed or removed tool?)", name)
				continue
			}
			if perm.IsReadOnly() {
				t.Errorf("MCP tool exemption %q names a tool that is already read-only "+
					"(covered by the verb rule) — the exemption is unnecessary", name)
			}
		}
	})

	// Assertion 5: every operation resolves to a category.
	t.Run("every operation resolves to a category", func(t *testing.T) {
		for _, op := range definedOps {
			switch op.Category {
			case audit.CategoryManagement, audit.CategoryAuthorization:
			default:
				t.Errorf("operation %q has category %q, want %q or %q",
					op.ID, op.Category, audit.CategoryManagement, audit.CategoryAuthorization)
			}
		}
	})

	// Assertion 6: no duplicate bindings. A Go map can't itself hold a
	// duplicate key, so the real guard is upstream: audit.MCPBindings and
	// audit.MergeMCPAliases each error on a (ToolName, Scope) collision
	// before the map is ever returned — the err check earlier in this test
	// would have failed had that happened. Cross-checking the bound-tool-name
	// count against the known total of 69 state-modifying tools (60
	// canonical + 9 deprecated aliases) catches the case a bare error check
	// wouldn't: a future refactor that swallows the collision error instead
	// of propagating it.
	t.Run("no duplicate bindings", func(t *testing.T) {
		const wantBoundToolNames = 69
		if len(boundToolNames) != wantBoundToolNames {
			t.Errorf("len(distinct bound tool names) = %d, want %d — a collision may have silently "+
				"dropped a tool, or a new tool needs a binding added", len(boundToolNames), wantBoundToolNames)
		}
	})

	// Assertion 7: every non-empty ResourceArg names a real argument in its
	// tool's actual input schema. RESTResourceParam gets this same check at
	// construction time (BuildPatternMap, pattern_map.go) because a typo'd
	// path parameter name is otherwise invisible until it silently produces
	// an empty resource.name on exactly the denied/failed events the seed
	// exists to populate. ResourceArg had no equivalent check — this closes
	// that gap the same way, against the schema a real MCP client sees.
	t.Run("every ResourceArg names a real tool argument", func(t *testing.T) {
		assertResourceArgsMatchToolSchemas(t, bindings, toolInputSchemaProperties(t))
	})
}

// assertResourceArgsMatchToolSchemas checks two things per binding: that its
// tool name is actually registered (closing finding 9 — a stale MCPToolName
// would otherwise be caught only indirectly, via the wantBoundToolNames
// pin), and that a non-empty ResourceArg names a property actually present
// in that tool's input schema. The registration check runs before the
// ResourceArg=="" early return, deliberately: create_workload,
// create_release_binding, create_workflow_run and the trigger_workflow_run
// alias all bind with an empty ResourceArg (see mcp_bindings.go), and a
// stale tool name on any of those four must still be caught here rather
// than silently passing this assertion.
//
// Split out from TestAuditCoverage's body to keep that function's own
// cyclomatic complexity under the gocyclo threshold.
func assertResourceArgsMatchToolSchemas(
	t *testing.T, bindings map[audit.MCPBindingKey]audit.MCPBinding, schemas map[string]map[string]bool,
) {
	t.Helper()
	for key, binding := range bindings {
		props, ok := schemas[key.ToolName]
		if !ok {
			t.Errorf("binding for tool %q (scope %q) references a tool that is not registered",
				key.ToolName, key.Scope)
			continue
		}
		if binding.ResourceArg == "" {
			continue
		}
		if !props[binding.ResourceArg] {
			t.Errorf("binding for tool %q (scope %q) declares ResourceArg %q, "+
				"but tool %q has no such argument", key.ToolName, key.Scope, binding.ResourceArg, key.ToolName)
		}
	}
}
