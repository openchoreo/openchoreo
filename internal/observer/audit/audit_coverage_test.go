// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// package audit_test, not audit: internal/observer/config imports this
// package, so a same-package test file here risks an import cycle through it.
// An external test package breaks that and still exercises every exported
// entry point.
package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/api/internalgen"
	observeraudit "github.com/openchoreo/openchoreo/internal/observer/audit"
	observermcp "github.com/openchoreo/openchoreo/internal/observer/mcp"
	coreaudit "github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// There is no test asserting that registered route patterns agree with the
// spec: both ports build their routers from the same generated, embedded
// spec that audit.BuildPatternMap also parses, so the two can never disagree.

// swaggers returns both of observer's generated specs. Coverage is about the
// whole served surface, so every assertion below spans both.
func swaggers(t *testing.T) []*openapi3.T {
	t.Helper()

	public, err := gen.GetSwagger()
	if err != nil {
		t.Fatalf("failed to load public swagger: %v", err)
	}
	internal, err := internalgen.GetSwagger()
	if err != nil {
		t.Fatalf("failed to load internal swagger: %v", err)
	}
	return []*openapi3.T{public, internal}
}

// stateModifyingOperationIDs returns every non-GET operationId across both
// live specs — the same universe tools/auditgen walks.
func stateModifyingOperationIDs(t *testing.T) map[string]bool {
	t.Helper()

	ids := make(map[string]bool)
	for _, swagger := range swaggers(t) {
		for _, path := range swagger.Paths.InMatchingOrder() {
			for method, op := range swagger.Paths.Find(path).Operations() {
				if method == "GET" {
					continue
				}
				ids[op.OperationID] = true
			}
		}
	}
	return ids
}

// TestAuditCoverage is a CI gate: every state-modifying observer REST
// operation must be audited or explicitly, reasoned-ly exempted, enforced at
// build time rather than left to drift silently.
func TestAuditCoverage(t *testing.T) {
	restOperationIDs := stateModifyingOperationIDs(t)
	definedOps := observeraudit.GetOperations()
	definedByID := make(map[string]coreaudit.Operation, len(definedOps))
	for _, op := range definedOps {
		definedByID[op.ID] = op
	}

	t.Run("every REST operation is defined or exempted", func(t *testing.T) {
		for id := range restOperationIDs {
			_, defined := definedByID[id]
			_, exempted := observeraudit.RESTExemptions[id]
			if !defined && !exempted {
				t.Errorf("operationId %q is neither audited (GetOperations) nor exempted "+
					"(RESTExemptions) — add one or the other", id)
			}
			if defined && exempted {
				t.Errorf("operationId %q is both audited and exempted — remove one", id)
			}
		}
	})

	t.Run("every exemption references a real state-modifying operation", func(t *testing.T) {
		for id, reason := range observeraudit.RESTExemptions {
			if reason == "" {
				t.Errorf("REST exemption %q has an empty reason", id)
			}
			if !restOperationIDs[id] {
				t.Errorf("REST exemption %q does not name a real state-modifying operation in either "+
					"live spec (renamed or removed operationId?)", id)
			}
		}
	})

	t.Run("every operation resolves to a category", func(t *testing.T) {
		for _, op := range definedOps {
			switch op.Category {
			case coreaudit.CategoryManagement, coreaudit.CategoryAuthorization:
			default:
				t.Errorf("operation %q has category %q, want %q or %q",
					op.ID, op.Category, coreaudit.CategoryManagement, coreaudit.CategoryAuthorization)
			}
		}
	})

	// Pins the total so a spec change is forced through a deliberate update
	// here rather than silently shifting the audited/exempted split.
	//
	// 13 = 9 public (14 less health, the OAuth metadata GET, both FinOps GETs
	// and the span-details GET) + 4 internal (5 less the getAlertRule GET).
	t.Run("total operation count is pinned", func(t *testing.T) {
		const wantTotal = 13
		if len(restOperationIDs) != wantTotal {
			t.Errorf("len(stateModifyingOperationIDs) = %d, want %d", len(restOperationIDs), wantTotal)
		}
	})
}

// TestEveryDefinedOperationBelongsToExactlyOneSpec closes the hole
// OperationsIn opens: an operation matching neither spec is dropped from both
// ports' pattern maps with no error and no event — which is what a renamed
// operationId without a regeneration would produce. Also catches the
// opposite, an ID declared by both specs.
func TestEveryDefinedOperationBelongsToExactlyOneSpec(t *testing.T) {
	specs := swaggers(t)
	names := []string{"observer-api.yaml", "observer-internal-api.yaml"}

	assigned := make(map[string][]string)
	for i, swagger := range specs {
		for _, op := range observeraudit.OperationsIn(swagger) {
			assigned[op.ID] = append(assigned[op.ID], names[i])
		}
	}

	for _, op := range observeraudit.GetOperations() {
		switch specsFor := assigned[op.ID]; len(specsFor) {
		case 1:
			// Exactly one port serves it — what the wiring assumes.
		case 0:
			t.Errorf("audited operation %q is declared by neither spec, so OperationsIn drops it from "+
				"both ports' pattern maps and it is silently never audited — regenerate "+
				"definitions.gen.go (make audit-gen)", op.ID)
		default:
			t.Errorf("audited operation %q is declared by %v — both ports would try to audit it",
				op.ID, specsFor)
		}
	}
}

// registeredMCPToolNames lists the tools observer's MCP server actually
// registers, read back over the protocol via an in-memory transport. The SDK
// exposes no way to enumerate tools off a *Server directly, so this connects a
// real client and issues tools/list.
func registeredMCPToolNames(t *testing.T) map[string]bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// A zero-valued handler: tools/list reads the registration metadata and
	// never invokes a handler, so no services are needed.
	server := observermcp.NewServer(&observermcp.MCPHandler{})

	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("failed to connect MCP server: %v", err)
	}
	defer func() { _ = serverSession.Close() }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "audit-coverage", Version: "0.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("failed to connect MCP client: %v", err)
	}
	defer func() { _ = clientSession.Close() }()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}

	names := make(map[string]bool, len(result.Tools))
	for _, tool := range result.Tools {
		names[tool.Name] = true
	}
	return names
}

// TestMCPToolRegistry_NoMutatingTools is a name pin, not a permission check:
// observer has no ToolPermission registry (unlike openchoreo-api's
// pkg/mcp/tools) to classify a tool as state-modifying from a declared authz
// action, so this cannot verify read-only-ness the way TestAuditCoverage's MCP
// assertions do for openchoreo-api. Every name below was confirmed read-only
// by reading registerTools.
//
// What it does enforce is that MCPToolNames matches what the server really
// registers. Adding a tool to registerTools without listing it here fails —
// which forces a human to classify the new tool rather than letting a mutating
// one ship unaudited by silent omission.
func TestMCPToolRegistry_NoMutatingTools(t *testing.T) {
	registered := registeredMCPToolNames(t)

	for name := range registered {
		if !observeraudit.MCPToolNames[name] {
			t.Errorf("tool %q is registered by internal/observer/mcp but missing from MCPToolNames — "+
				"add it after confirming it is read-only, or wire MCP audit if it is not", name)
		}
	}
	for name := range observeraudit.MCPToolNames {
		if !registered[name] {
			t.Errorf("MCPToolNames lists %q, which internal/observer/mcp no longer registers — "+
				"remove it", name)
		}
	}
}
