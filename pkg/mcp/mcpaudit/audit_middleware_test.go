// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcpaudit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// recordingSink is a test double that records every Event it receives.
type recordingSink struct {
	events []*audit.Event
}

func (s *recordingSink) LogEvent(event *audit.Event) {
	s.events = append(s.events, event)
}

func testBindings() map[audit.MCPBindingKey]audit.MCPBinding {
	op := &audit.Operation{
		ID: "CreateProject", Action: "create_project", ResourceType: "projects", Category: audit.CategoryManagement,
	}
	return map[audit.MCPBindingKey]audit.MCPBinding{
		{ToolName: "create_project"}: {Operation: op, ResourceArg: "name"},
	}
}

func testEmitter(t *testing.T, sink audit.Sink) *audit.Emitter {
	t.Helper()
	policies, errs := audit.NewPolicySet(coreconfig.NewPath("audit"), audit.Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	emitter, err := audit.NewEmitter("test-service", policies, sink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return emitter
}

// testMiddleware builds the audit middleware for a test, failing loudly if
// opts is misconfigured — real production callers must check the same error
// (see pkg/mcp's NewHTTPServer).
func testMiddleware(t *testing.T, opts MiddlewareOptions) mcp.Middleware {
	t.Helper()
	mw, err := NewMiddleware(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return mw
}

// TestResolveBinding covers the 0.10c scope-resolution logic: an unscoped
// tool resolves on the first (fast-path) lookup with no scope argument
// needed; a scope-collapsed tool resolves on the second lookup, keyed by the
// call's "scope" argument, defaulting an absent or empty value to
// tools.ScopeNamespace to match resolveScope's own default in pkg/mcp/tools.
func TestResolveBinding(t *testing.T) {
	unscopedOp := &audit.Operation{ID: "CreateProject", Action: "create_project", Category: audit.CategoryManagement}
	nsOp := &audit.Operation{
		ID: "CreateComponentType", Action: "create_component_type", Category: audit.CategoryManagement,
	}
	clusterOp := &audit.Operation{
		ID: "CreateClusterComponentType", Action: "create_cluster_component_type", Category: audit.CategoryManagement,
	}
	bindings := map[audit.MCPBindingKey]audit.MCPBinding{
		{ToolName: "create_project"}:                                     {Operation: unscopedOp, ResourceArg: "name"},
		{ToolName: "create_component_type", Scope: tools.ScopeNamespace}: {Operation: nsOp, ResourceArg: "name"},
		{ToolName: "create_component_type", Scope: tools.ScopeCluster}:   {Operation: clusterOp, ResourceArg: "name"},
	}

	reqWithScope := func(toolName, scope string) mcp.Request {
		args := map[string]any{}
		if scope != "" {
			args["scope"] = scope
		}
		raw, _ := json.Marshal(args)
		return &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{Name: toolName, Arguments: raw}}
	}

	tests := []struct {
		name      string
		req       mcp.Request
		wantBound bool
		wantOpID  string
	}{
		{
			name: "unscoped tool resolves on the fast path", req: reqWithScope("create_project", ""),
			wantBound: true, wantOpID: "CreateProject",
		},
		{
			name: "scope-collapsed tool, explicit namespace scope", req: reqWithScope("create_component_type", "namespace"),
			wantBound: true, wantOpID: "CreateComponentType",
		},
		{
			name: "scope-collapsed tool, explicit cluster scope", req: reqWithScope("create_component_type", "cluster"),
			wantBound: true, wantOpID: "CreateClusterComponentType",
		},
		{
			name: "scope-collapsed tool, absent scope defaults to namespace", req: reqWithScope("create_component_type", ""),
			wantBound: true, wantOpID: "CreateComponentType",
		},
		{name: "unbound tool", req: reqWithScope("some_other_tool", ""), wantBound: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, bound := resolveBinding(bindings, callToolName(tt.req), tt.req)
			if bound != tt.wantBound {
				t.Fatalf("bound = %v, want %v", bound, tt.wantBound)
			}
			if !tt.wantBound {
				return
			}
			if b.Operation == nil || b.Operation.ID != tt.wantOpID {
				t.Errorf("Operation.ID = %v, want %q", b.Operation, tt.wantOpID)
			}
		})
	}
}

// TestNewMiddleware_EmitsOnPanic mirrors REST's TestMiddleware_Handler_EmitsOnPanic:
// a panicking tool call must still produce an audit record, then re-panic.
func TestNewMiddleware_EmitsOnPanic(t *testing.T) {
	sink := &recordingSink{}
	emitter := testEmitter(t, sink)
	mw := testMiddleware(t, MiddlewareOptions{Emitter: emitter, Bindings: testBindings(), Enabled: true})

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

// TestNewMiddleware_PassesThroughWithoutAuditing covers the two reasons a
// tools/call can skip audit entirely: the middleware is disabled (guards
// against a repeat of the bug where audit.enabled=false silenced REST but
// MCP kept emitting), or the tool isn't in the binding table. Either way,
// next must still run and no event must be emitted.
func TestNewMiddleware_PassesThroughWithoutAuditing(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		toolName string
	}{
		{name: "disabled", enabled: false, toolName: "create_project"},
		{name: "unbound tool", enabled: true, toolName: "some_unbound_tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := &recordingSink{}
			emitter := testEmitter(t, sink)
			mw := testMiddleware(t, MiddlewareOptions{Emitter: emitter, Bindings: testBindings(), Enabled: tt.enabled})

			called := false
			next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
				called = true
				return &mcp.CallToolResult{}, nil
			}

			req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{Name: tt.toolName}}

			if _, err := mw(next)(context.Background(), methodCallTool, req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !called {
				t.Fatal("expected next to be called")
			}
			if len(sink.events) != 0 {
				t.Errorf("expected no audit events, got %d", len(sink.events))
			}
		})
	}
}

func TestNewMiddleware_NonCallToolMethodPassesThrough(t *testing.T) {
	sink := &recordingSink{}
	emitter := testEmitter(t, sink)
	mw := testMiddleware(t, MiddlewareOptions{Emitter: emitter, Bindings: testBindings(), Enabled: true})

	called := false
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		called = true
		return nil, nil
	}

	req := &mcp.ServerRequest[*mcp.PingParams]{Params: &mcp.PingParams{}}

	if _, err := mw(next)(context.Background(), "ping", req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected next to be called for a non-tools/call method")
	}
	if len(sink.events) != 0 {
		t.Errorf("expected no audit events for a non-tools/call method, got %d", len(sink.events))
	}
}

// TestNewMiddleware_SuccessEmitsSuccessResult exercises the non-panicking,
// non-disabled, bound-tool path end to end — the one classifyResult branch
// (nil error, non-error result) none of the other tests reach.
func TestNewMiddleware_SuccessEmitsSuccessResult(t *testing.T) {
	sink := &recordingSink{}
	emitter := testEmitter(t, sink)
	mw := testMiddleware(t, MiddlewareOptions{Emitter: emitter, Bindings: testBindings(), Enabled: true})

	next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	}

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: "create_project", Arguments: json.RawMessage(`{"name":"proj-1"}`)},
	}

	if _, err := mw(next)(context.Background(), methodCallTool, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("expected exactly one audit event, got %d", len(sink.events))
	}
	if sink.events[0].Result != audit.ResultSuccess {
		t.Errorf("Result = %v, want success", sink.events[0].Result)
	}
	if sink.events[0].Resource == nil || sink.events[0].Resource.Name != "proj-1" {
		t.Errorf("Resource = %+v, want Name proj-1 (seeded from the resource arg)", sink.events[0].Resource)
	}
}

func TestClassifyResult(t *testing.T) {
	tests := []struct {
		name string
		res  mcp.Result
		err  error
		want audit.Result
	}{
		{name: "no error, non-error result is success", res: &mcp.CallToolResult{}, err: nil, want: audit.ResultSuccess},
		{name: "no error, nil result is success", res: nil, err: nil, want: audit.ResultSuccess},
		{
			name: "no error, IsError result is failure",
			res:  &mcp.CallToolResult{IsError: true}, err: nil, want: audit.ResultFailure,
		},
		{name: "ErrNoSubject is denied", res: nil, err: tools.ErrNoSubject, want: audit.ResultDenied},
		{name: "ErrForbidden is denied", res: nil, err: tools.ErrForbidden, want: audit.ResultDenied},
		{name: "ErrPDPFailure is failure, not denied", res: nil, err: tools.ErrPDPFailure, want: audit.ResultFailure},
		{
			name: "wrapped ErrForbidden is still denied", res: nil,
			err: errors.Join(errors.New("ctx"), tools.ErrForbidden), want: audit.ResultDenied,
		},
		{name: "unrelated error is failure", res: nil, err: errors.New("boom"), want: audit.ResultFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyResult(tt.res, tt.err); got != tt.want {
				t.Errorf("classifyResult(%v, %v) = %v, want %v", tt.res, tt.err, got, tt.want)
			}
		})
	}
}

func TestCallToolName(t *testing.T) {
	if got := callToolName(nil); got != "" {
		t.Errorf("callToolName(nil) = %q, want empty", got)
	}

	wrongParams := &mcp.ServerRequest[*mcp.PingParams]{Params: &mcp.PingParams{}}
	if got := callToolName(wrongParams); got != "" {
		t.Errorf("callToolName(non-CallTool request) = %q, want empty", got)
	}

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{Name: "create_project"}}
	if got := callToolName(req); got != "create_project" {
		t.Errorf("callToolName(req) = %q, want create_project", got)
	}
}

func TestExtractResourceArg(t *testing.T) {
	tests := []struct {
		name    string
		req     mcp.Request
		argName string
		want    string
	}{
		{name: "nil request", req: nil, argName: "name", want: ""},
		{
			name: "empty argName", req: &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{}},
			argName: "", want: "",
		},
		{
			name: "wrong params type", req: &mcp.ServerRequest[*mcp.PingParams]{Params: &mcp.PingParams{}},
			argName: "name", want: "",
		},
		{
			name: "no arguments", req: &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{}},
			argName: "name", want: "",
		},
		{
			name: "malformed JSON arguments",
			req: &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
				Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{not json`)},
			},
			argName: "name", want: "",
		},
		{
			name: "arg present but not a string",
			req: &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
				Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"name":123}`)},
			},
			argName: "name", want: "",
		},
		{
			name: "arg absent",
			req: &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
				Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"other":"x"}`)},
			},
			argName: "name", want: "",
		},
		{
			name: "arg present as string",
			req: &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
				Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"name":"proj-1"}`)},
			},
			argName: "name", want: "proj-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractResourceArg(tt.req, tt.argName); got != tt.want {
				t.Errorf("extractResourceArg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRequestHeader(t *testing.T) {
	if got := requestHeader(nil); got != nil {
		t.Errorf("requestHeader(nil) = %v, want nil", got)
	}

	noExtra := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{}}
	if got := requestHeader(noExtra); got != nil {
		t.Errorf("requestHeader(no Extra) = %v, want nil", got)
	}

	want := http.Header{}
	want.Set("X-Request-ID", "abc")
	withExtra := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{},
		Extra:  &mcp.RequestExtra{Header: want},
	}
	if got := requestHeader(withExtra); got.Get("X-Request-ID") != "abc" {
		t.Errorf("requestHeader(withExtra) = %v, want header carrying X-Request-ID=abc", got)
	}
}
