// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcpaudit

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// recordingSink is a test double that records every Event it receives.
type recordingSink struct {
	events []*audit.Event
}

func (s *recordingSink) LogEvent(event *audit.Event) {
	s.events = append(s.events, event)
}

func testBindings() map[string]audit.MCPBinding {
	op := &audit.Operation{
		ID: "CreateProject", Action: "create_project", ResourceType: "projects", Category: audit.CategoryManagement,
	}
	return map[string]audit.MCPBinding{"create_project": {Operation: op, ResourceArg: "name"}}
}

func testEmitter(t *testing.T, sink audit.Sink) *audit.Emitter {
	t.Helper()
	policies, errs := audit.NewPolicySet(coreconfig.NewPath("audit"), audit.Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	return audit.NewEmitter("test-service", policies, sink)
}

// TestNewMiddleware_EmitsOnPanic mirrors REST's TestMiddleware_Handler_EmitsOnPanic:
// a panicking tool call must still produce an audit record, then re-panic.
func TestNewMiddleware_EmitsOnPanic(t *testing.T) {
	sink := &recordingSink{}
	emitter := testEmitter(t, sink)
	mw := NewMiddleware(MiddlewareOptions{Emitter: emitter, Bindings: testBindings(), Enabled: true})

	panicking := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		panic("handler blew up after mutating state")
	}

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: "create_project"},
	}

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected the panic to propagate, got none")
			}
		}()
		_, _ = mw(panicking)(context.Background(), methodCallTool, req)
	}()

	if len(sink.events) != 1 {
		t.Fatalf("expected exactly one audit event despite the panic, got %d", len(sink.events))
	}
	if sink.events[0].Result != audit.ResultFailure {
		t.Errorf("Result = %v, want failure", sink.events[0].Result)
	}
}

// TestNewMiddleware_DisabledSkipsAllAuditLogic guards against a repeat of the
// bug where audit.enabled=false silenced REST but MCP kept emitting: with
// Enabled: false, a bound tool call must pass straight through with no event
// emitted.
func TestNewMiddleware_DisabledSkipsAllAuditLogic(t *testing.T) {
	sink := &recordingSink{}
	emitter := testEmitter(t, sink)
	mw := NewMiddleware(MiddlewareOptions{Emitter: emitter, Bindings: testBindings(), Enabled: false})

	called := false
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		called = true
		return &mcp.CallToolResult{}, nil
	}

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: "create_project"},
	}

	if _, err := mw(next)(context.Background(), methodCallTool, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected next to be called even when audit is disabled")
	}
	if len(sink.events) != 0 {
		t.Errorf("expected no audit events when disabled, got %d", len(sink.events))
	}
}
