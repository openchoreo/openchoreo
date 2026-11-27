// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	authzcasbin "github.com/openchoreo/openchoreo/internal/authz/casbin"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// ---------------------------------------------------------------------------
// Helpers / mocks
// ---------------------------------------------------------------------------

// mustNewToolFilterMiddleware builds a middleware for tests that don't care
// about the logger requirement itself (see TestNewToolFilterMiddleware_ErrorsOnNilLogger
// for that case) and fails the test immediately on an unexpected error.
func mustNewToolFilterMiddleware(
	t *testing.T, logger *slog.Logger, pdp authzcore.PDP,
	perms map[string]ToolPermission, toolToToolsets map[string]map[ToolsetType]bool,
) mcp.Middleware {
	t.Helper()
	mw, err := NewToolFilterMiddleware(logger, pdp, perms, toolToToolsets)
	if err != nil {
		t.Fatalf("NewToolFilterMiddleware: %v", err)
	}
	return mw
}

// mockPDP is a test double for authzcore.PDP.
type mockPDP struct {
	profile *authzcore.UserCapabilitiesResponse
	err     error
}

func (m *mockPDP) Evaluate(_ context.Context, _ *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	return &authzcore.Decision{Decision: true}, nil
}

func (m *mockPDP) BatchEvaluate(
	_ context.Context, _ *authzcore.BatchEvaluateRequest,
) (*authzcore.BatchEvaluateResponse, error) {
	return &authzcore.BatchEvaluateResponse{}, nil
}

func (m *mockPDP) GetSubjectProfile(
	_ context.Context, _ *authzcore.ProfileRequest,
) (*authzcore.UserCapabilitiesResponse, error) {
	return m.profile, m.err
}

// allowAllProfile returns a UserCapabilitiesResponse that grants every provided action.
func allowAllProfile(actions ...string) *authzcore.UserCapabilitiesResponse {
	caps := make(map[string]*authzcore.ActionCapability, len(actions))
	for _, a := range actions {
		caps[a] = &authzcore.ActionCapability{
			Allowed: []*authzcore.CapabilityResource{
				{Path: "namespace/test"},
			},
		}
	}
	return &authzcore.UserCapabilitiesResponse{Capabilities: caps}
}

// denyAllProfile returns a profile with no capabilities.
func denyAllProfile() *authzcore.UserCapabilitiesResponse {
	return &authzcore.UserCapabilitiesResponse{Capabilities: map[string]*authzcore.ActionCapability{}}
}

// ctxWithSubject injects a SubjectContext into the context, simulating the JWT middleware.
func ctxWithSubject(ctx context.Context) context.Context {
	return auth.SetSubjectContext(ctx, &auth.SubjectContext{
		ID:                "user-1",
		Type:              "user",
		EntitlementClaim:  "groups",
		EntitlementValues: []string{"devs"},
	})
}

// ---------------------------------------------------------------------------
// Middleware unit tests
// ---------------------------------------------------------------------------

// TestNewToolFilterMiddleware_ErrorsOnNilLogger guards the fail-loudly
// contract: a missing logger must fail construction, not silently fall back
// to slog.Default() and risk routing PDP-failure detail somewhere nobody is
// watching.
func TestNewToolFilterMiddleware_ErrorsOnNilLogger(t *testing.T) {
	_, err := NewToolFilterMiddleware(nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected an error for a nil logger, got nil")
	}
}

// TestNewToolFilterMiddlewareNilPDP verifies that when pdp is nil all tools are
// returned unfiltered (graceful degradation when authz is disabled).
func TestNewToolFilterMiddlewareNilPDP(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_namespaces"}, func(
		ctx context.Context, req *mcp.CallToolRequest, args any,
	) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})

	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), nil, map[string]ToolPermission{
		"list_namespaces": {ToolName: "list_namespaces", Action: "namespace:view"},
	}, nil))

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	_, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(result.Tools) != 1 {
		t.Errorf("expected 1 tool when PDP is nil, got %d", len(result.Tools))
	}
}

// TestToolFilterMiddlewareFiltersListTools verifies that tools/list only returns
// tools the user has permission to use.
func TestToolFilterMiddlewareFiltersListTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_namespaces"}, nopToolHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "create_namespace"}, nopToolHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "list_projects"}, nopToolHandler)

	perms := map[string]ToolPermission{
		"list_namespaces":  {ToolName: "list_namespaces", Action: "namespace:view"},
		"create_namespace": {ToolName: "create_namespace", Action: "namespace:create"},
		"list_projects":    {ToolName: "list_projects", Action: "project:view"},
	}

	// User can only view namespaces and projects, not create namespace.
	pdp := &mockPDP{profile: allowAllProfile("namespace:view", "project:view")}
	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), pdp, perms, nil))

	ctx := ctxWithSubject(context.Background())
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	_, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	found := make(map[string]bool)
	for _, tool := range result.Tools {
		found[tool.Name] = true
	}

	if !found["list_namespaces"] {
		t.Error("expected list_namespaces to be visible")
	}
	if !found["list_projects"] {
		t.Error("expected list_projects to be visible")
	}
	if found["create_namespace"] {
		t.Error("create_namespace should not be visible: user lacks namespace:create")
	}
}

// TestToolFilterMiddlewareDenyAllProfile verifies that when a user has no capabilities,
// no tools are returned.
func TestToolFilterMiddlewareDenyAllProfile(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_namespaces"}, nopToolHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "create_namespace"}, nopToolHandler)

	perms := map[string]ToolPermission{
		"list_namespaces":  {ToolName: "list_namespaces", Action: "namespace:view"},
		"create_namespace": {ToolName: "create_namespace", Action: "namespace:create"},
	}

	pdp := &mockPDP{profile: denyAllProfile()}
	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), pdp, perms, nil))

	ctx := ctxWithSubject(context.Background())
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	_, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	if len(result.Tools) != 0 {
		t.Errorf("expected 0 tools for user with no capabilities, got %d", len(result.Tools))
	}
}

// TestToolFilterMiddlewareNoSubjectInContext verifies that when there is no
// authenticated user in context, tools/list returns an empty list.
func TestToolFilterMiddlewareNoSubjectInContext(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_namespaces"}, nopToolHandler)

	perms := map[string]ToolPermission{
		"list_namespaces": {ToolName: "list_namespaces", Action: "namespace:view"},
	}

	pdp := &mockPDP{profile: allowAllProfile("namespace:view")}
	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), pdp, perms, nil))

	// Context WITHOUT a subject.
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	_, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	if len(result.Tools) != 0 {
		t.Errorf("expected 0 tools when no subject in context, got %d", len(result.Tools))
	}
}

// TestToolFilterMiddlewarePDPError verifies graceful degradation when the PDP
// returns an error: no tools are shown.
func TestToolFilterMiddlewarePDPError(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_namespaces"}, nopToolHandler)

	perms := map[string]ToolPermission{
		"list_namespaces": {ToolName: "list_namespaces", Action: "namespace:view"},
	}

	pdp := &mockPDP{err: errors.New("pdp unavailable")}
	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), pdp, perms, nil))

	ctx := ctxWithSubject(context.Background())
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	_, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	if len(result.Tools) != 0 {
		t.Errorf("expected 0 tools when PDP errors, got %d", len(result.Tools))
	}
}

// TestToolFilterMiddlewareFailClosedListIsStale verifies that a tools/list
// emptied by a missing subject or a PDP error carries ttlMs 0, so clients do
// not cache it, while a filtered list keeps the server's TTL.
func TestToolFilterMiddlewareFailClosedListIsStale(t *testing.T) {
	const ttlMs = 60_000
	perms := map[string]ToolPermission{
		"list_namespaces": {ToolName: "list_namespaces", Action: "namespace:view"},
	}
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.ListToolsResult{
			Cacheable: mcp.Cacheable{TTLMs: ttlMs, CacheScope: "private"},
			Tools:     []*mcp.Tool{{Name: "list_namespaces"}},
		}, nil
	}
	tests := []struct {
		name      string
		ctx       context.Context
		pdp       *mockPDP
		wantTools int
		wantTTLMs int
	}{
		{"no subject", context.Background(), &mockPDP{profile: allowAllProfile("namespace:view")}, 0, 0},
		{"PDP error", ctxWithSubject(context.Background()), &mockPDP{err: errors.New("pdp unavailable")}, 0, 0},
		{"allowed", ctxWithSubject(context.Background()), &mockPDP{profile: allowAllProfile("namespace:view")}, 1, ttlMs},
		{"denied by policy", ctxWithSubject(context.Background()), &mockPDP{profile: denyAllProfile()}, 0, ttlMs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := mustNewToolFilterMiddleware(t, slog.Default(), tt.pdp, perms, nil)
			res, err := mw(next)(tt.ctx, methodListTools, &mcp.ListToolsRequest{Params: &mcp.ListToolsParams{}})
			if err != nil {
				t.Fatalf("tools/list: %v", err)
			}
			list := res.(*mcp.ListToolsResult)
			if len(list.Tools) != tt.wantTools || list.TTLMs != tt.wantTTLMs {
				t.Errorf("tools/ttlMs = %d/%d, want %d/%d", len(list.Tools), list.TTLMs, tt.wantTools, tt.wantTTLMs)
			}
			if list.CacheScope != "private" {
				t.Errorf("cacheScope = %q, want private", list.CacheScope)
			}
		})
	}
}

// TestToolFilterMiddlewareCallToolAllowed verifies that an authorized user can
// call a tool.
func TestToolFilterMiddlewareCallToolAllowed(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	called := false
	mcp.AddTool(server, &mcp.Tool{Name: "list_namespaces"}, func(
		ctx context.Context, req *mcp.CallToolRequest, args any,
	) (*mcp.CallToolResult, any, error) {
		called = true
		return &mcp.CallToolResult{}, nil, nil
	})

	perms := map[string]ToolPermission{
		"list_namespaces": {ToolName: "list_namespaces", Action: "namespace:view"},
	}

	pdp := &mockPDP{profile: allowAllProfile("namespace:view")}
	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), pdp, perms, nil))

	ctx := ctxWithSubject(context.Background())
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	_, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	_, err = clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "list_namespaces"})
	if err != nil {
		t.Fatalf("CallTool: expected success, got %v", err)
	}
	if !called {
		t.Error("expected tool handler to be called, but it was not")
	}
}

// TestToolFilterMiddlewareCallToolDenied verifies that an unauthorized user gets
// an error when attempting to call a tool they lack permission for.
func TestToolFilterMiddlewareCallToolDenied(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	called := false
	mcp.AddTool(server, &mcp.Tool{Name: "create_namespace"}, func(
		ctx context.Context, req *mcp.CallToolRequest, args any,
	) (*mcp.CallToolResult, any, error) {
		called = true
		return &mcp.CallToolResult{}, nil, nil
	})

	perms := map[string]ToolPermission{
		"create_namespace": {ToolName: "create_namespace", Action: "namespace:create"},
	}

	// User only has view, not create.
	pdp := &mockPDP{profile: allowAllProfile("namespace:view")}
	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), pdp, perms, nil))

	ctx := ctxWithSubject(context.Background())
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	_, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	_, err = clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "create_namespace"})
	if err == nil {
		t.Error("expected error when calling create_namespace without permission, got nil")
	}
	if called {
		t.Error("tool handler should not have been called for unauthorized user")
	}
}

// TestToolFilterMiddlewareUnknownToolPassthrough verifies that a tool with no
// permission entry passes through unfiltered (safe default).
func TestToolFilterMiddlewareUnknownToolPassthrough(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "some_new_tool"}, nopToolHandler)

	// Empty perms — "some_new_tool" has no entry.
	perms := map[string]ToolPermission{}

	pdp := &mockPDP{profile: denyAllProfile()}
	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), pdp, perms, nil))

	ctx := ctxWithSubject(context.Background())
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	_, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	if len(result.Tools) != 1 {
		t.Errorf("expected tool with no perm entry to be visible, got %d tools", len(result.Tools))
	}
}

// nopToolHandler is a minimal tool handler used in tests.
func nopToolHandler(ctx context.Context, req *mcp.CallToolRequest, args any) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{}, nil, nil
}

// ---------------------------------------------------------------------------
// Toolset narrowing — ?toolsets= behavior
// ---------------------------------------------------------------------------

// TestToolFilterMiddlewareToolsetNarrowing verifies that when the client
// requests a subset of toolsets via context, tools/list returns only tools
// whose toolset is in the requested set.
func TestToolFilterMiddlewareToolsetNarrowing(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_namespaces"}, nopToolHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "list_components"}, nopToolHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "create_environment"}, nopToolHandler)

	toolToToolsets := map[string]map[ToolsetType]bool{
		"list_namespaces":    {ToolsetNamespace: true},
		"list_components":    {ToolsetComponent: true},
		"create_environment": {ToolsetPE: true},
	}

	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), nil, nil, toolToToolsets))

	// Client requested only the namespace and pe toolsets.
	ctx := WithRequestedToolsets(context.Background(), map[ToolsetType]bool{
		ToolsetNamespace: true,
		ToolsetPE:        true,
	})

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	got := map[string]bool{}
	for _, tool := range result.Tools {
		got[tool.Name] = true
	}
	if !got["list_namespaces"] {
		t.Error("expected list_namespaces (namespace toolset) to be visible")
	}
	if !got["create_environment"] {
		t.Error("expected create_environment (pe toolset) to be visible")
	}
	if got["list_components"] {
		t.Error("list_components (component toolset) should be hidden when narrowed to namespace+pe")
	}
}

// TestToolFilterMiddlewareToolsetNarrowingMultiOwner verifies that a tool
// registered by more than one toolset (such as list_component_types, which is
// shared between component and pe) is visible when *any* of its owning
// toolsets is requested.
func TestToolFilterMiddlewareToolsetNarrowingMultiOwner(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_component_types"}, nopToolHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "list_namespaces"}, nopToolHandler)

	toolToToolsets := map[string]map[ToolsetType]bool{
		"list_component_types": {ToolsetComponent: true, ToolsetPE: true},
		"list_namespaces":      {ToolsetNamespace: true},
	}

	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), nil, nil, toolToToolsets))

	// Narrow to pe only — list_component_types is co-owned by pe so it should still appear.
	ctx := WithRequestedToolsets(context.Background(), map[ToolsetType]bool{ToolsetPE: true})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	got := map[string]bool{}
	for _, tool := range result.Tools {
		got[tool.Name] = true
	}
	if !got["list_component_types"] {
		t.Error("expected list_component_types (co-owned by pe) to be visible when narrowed to pe")
	}
	if got["list_namespaces"] {
		t.Error("list_namespaces (namespace toolset only) should be hidden when narrowed to pe")
	}
}

// TestToolFilterMiddlewareToolsetNarrowingUnknownIgnored verifies that an
// unknown toolset name in the requested set silently matches no tools rather
// than returning an error or hiding everything spuriously.
func TestToolFilterMiddlewareToolsetNarrowingUnknownIgnored(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_namespaces"}, nopToolHandler)

	toolToToolsets := map[string]map[ToolsetType]bool{
		"list_namespaces": {ToolsetNamespace: true},
	}

	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), nil, nil, toolToToolsets))

	// Request a toolset name that no registered tool belongs to.
	ctx := WithRequestedToolsets(context.Background(), map[ToolsetType]bool{ToolsetType("does-not-exist"): true})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(result.Tools) != 0 {
		t.Errorf("expected no tools when only an unknown toolset is requested, got %d", len(result.Tools))
	}
}

// TestToolFilterMiddlewareToolsetAndAuthzCombined verifies that toolset
// narrowing and authz filtering compose: the visible set is the intersection
// of the two filters.
func TestToolFilterMiddlewareToolsetAndAuthzCombined(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_namespaces"}, nopToolHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "create_namespace"}, nopToolHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "list_components"}, nopToolHandler)

	perms := map[string]ToolPermission{
		"list_namespaces":  {ToolName: "list_namespaces", Action: "namespace:view"},
		"create_namespace": {ToolName: "create_namespace", Action: "namespace:create"},
		"list_components":  {ToolName: "list_components", Action: "component:view"},
	}
	toolToToolsets := map[string]map[ToolsetType]bool{
		"list_namespaces":  {ToolsetNamespace: true},
		"create_namespace": {ToolsetNamespace: true},
		"list_components":  {ToolsetComponent: true},
	}

	pdp := &mockPDP{profile: allowAllProfile("namespace:view", "component:view")}
	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), pdp, perms, toolToToolsets))

	// Narrow to namespace only — list_components must be hidden by toolset filter,
	// and create_namespace must be hidden by authz filter.
	ctx := WithRequestedToolsets(ctxWithSubject(context.Background()), map[ToolsetType]bool{ToolsetNamespace: true})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := map[string]bool{}
	for _, tool := range result.Tools {
		got[tool.Name] = true
	}
	if !got["list_namespaces"] {
		t.Error("expected list_namespaces to be visible (in namespace toolset and user has namespace:view)")
	}
	if got["list_components"] {
		t.Error("list_components should be hidden by toolset narrowing")
	}
	if got["create_namespace"] {
		t.Error("create_namespace should be hidden by authz (user lacks namespace:create)")
	}
}

// ---------------------------------------------------------------------------
// filterByAuthz=false bypass
// ---------------------------------------------------------------------------

// TestToolFilterMiddlewareFilterByAuthzFalseListTools verifies that when the
// client opts out of MCP-layer authz filtering, all tools are returned via
// tools/list regardless of the user's permissions.
func TestToolFilterMiddlewareFilterByAuthzFalseListTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_namespaces"}, nopToolHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "create_namespace"}, nopToolHandler)

	perms := map[string]ToolPermission{
		"list_namespaces":  {ToolName: "list_namespaces", Action: "namespace:view"},
		"create_namespace": {ToolName: "create_namespace", Action: "namespace:create"},
	}

	pdp := &mockPDP{profile: denyAllProfile()}
	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), pdp, perms, nil))

	ctx := WithFilterByAuthz(ctxWithSubject(context.Background()), false)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(result.Tools) != 2 {
		t.Errorf("expected 2 tools when filterByAuthz=false bypasses authz, got %d", len(result.Tools))
	}
}

// TestToolFilterMiddlewareFilterByAuthzFalseAllowsCall verifies that when the
// client opts out of MCP-layer authz filtering, tools/call is allowed without
// the middleware checking the user's permissions. The service layer is still
// expected to enforce authz independently.
func TestToolFilterMiddlewareFilterByAuthzFalseAllowsCall(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	called := false
	mcp.AddTool(server, &mcp.Tool{Name: "create_namespace"}, func(
		ctx context.Context, req *mcp.CallToolRequest, args any,
	) (*mcp.CallToolResult, any, error) {
		called = true
		return &mcp.CallToolResult{}, nil, nil
	})

	perms := map[string]ToolPermission{
		"create_namespace": {ToolName: "create_namespace", Action: "namespace:create"},
	}
	pdp := &mockPDP{profile: denyAllProfile()}
	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), pdp, perms, nil))

	ctx := WithFilterByAuthz(ctxWithSubject(context.Background()), false)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	if _, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "create_namespace"}); err != nil {
		t.Fatalf("CallTool: expected success when filterByAuthz=false, got %v", err)
	}
	if !called {
		t.Error("expected tool handler to be called when filterByAuthz=false")
	}
}

// TestToolFilterMiddlewareToolsetNarrowingDoesNotGateCall verifies that
// requesting a narrow toolset does not gate tools/call: a client that knows
// the tool name can still invoke any registered tool. (The filter is a
// tools/list visibility helper only.)
func TestToolFilterMiddlewareToolsetNarrowingDoesNotGateCall(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	called := false
	mcp.AddTool(server, &mcp.Tool{Name: "list_components"}, func(
		ctx context.Context, req *mcp.CallToolRequest, args any,
	) (*mcp.CallToolResult, any, error) {
		called = true
		return &mcp.CallToolResult{}, nil, nil
	})

	toolToToolsets := map[string]map[ToolsetType]bool{
		"list_components": {ToolsetComponent: true},
	}

	server.AddReceivingMiddleware(mustNewToolFilterMiddleware(t, slog.Default(), nil, nil, toolToToolsets))

	// Narrow to namespace toolset — list_components is component, but call should still succeed.
	ctx := WithRequestedToolsets(context.Background(), map[ToolsetType]bool{ToolsetNamespace: true})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	if _, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "list_components"}); err != nil {
		t.Fatalf("CallTool: expected success despite toolset narrowing, got %v", err)
	}
	if !called {
		t.Error("expected tool handler to be called — toolset narrowing only applies to tools/list")
	}
}

// TestFilterCallTool_SentinelClassification exercises filterCallTool directly
// (bypassing the JSON-RPC transport, which would erase the Go error chain) to
// confirm its three denial sites wrap distinct sentinels: a PDP outage
// (ErrPDPFailure) must be classifiable as an infrastructure failure, never as
// if the user had actually been denied by policy (ErrForbidden).
func TestFilterCallTool_SentinelClassification(t *testing.T) {
	perms := map[string]ToolPermission{
		"create_namespace": {ToolName: "create_namespace", Action: "namespace:create"},
	}
	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: "create_namespace"},
	}
	next := func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		t.Fatal("next should not be called when the call is denied")
		return nil, nil
	}

	t.Run("no subject wraps ErrNoSubject", func(t *testing.T) {
		pdp := &mockPDP{profile: allowAllProfile("namespace:create")}
		_, err := filterCallTool(context.Background(), next, methodCallTool, req, pdp, perms, slog.Default())
		if !errors.Is(err, ErrNoSubject) {
			t.Errorf("err = %v, want it to wrap ErrNoSubject", err)
		}
	})

	t.Run("PDP failure wraps ErrPDPFailure, not ErrForbidden", func(t *testing.T) {
		pdp := &mockPDP{err: errors.New("pdp unavailable")}
		_, err := filterCallTool(ctxWithSubject(context.Background()), next, methodCallTool, req, pdp, perms, slog.Default())
		if !errors.Is(err, ErrPDPFailure) {
			t.Errorf("err = %v, want it to wrap ErrPDPFailure", err)
		}
		if errors.Is(err, ErrForbidden) {
			t.Errorf("err = %v, a PDP outage must not be classified as ErrForbidden", err)
		}
	})

	// TestFilterCallTool_SentinelClassification/PDP failure does not leak the
	// raw PDP error text guards against an internal error (network detail,
	// internal endpoint, etc.) reaching the MCP client, which go-sdk would
	// relay back as tool-call result content. Sanitization happens in
	// NewToolFilterMiddleware's closure (where logger is bound), not in
	// filterCallTool itself, so this goes through the full middleware rather
	// than calling filterCallTool directly.
	t.Run("PDP failure does not leak the raw PDP error text", func(t *testing.T) {
		rawErr := "dial tcp 10.0.0.5:8443: connect: connection refused"
		pdp := &mockPDP{err: errors.New(rawErr)}
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

		mw := mustNewToolFilterMiddleware(t, logger, pdp, perms, nil)
		_, err := mw(next)(ctxWithSubject(context.Background()), methodCallTool, req)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if strings.Contains(err.Error(), rawErr) {
			t.Errorf("err = %q, must not contain the raw PDP error text %q", err.Error(), rawErr)
		}
		if !strings.Contains(logBuf.String(), rawErr) {
			t.Errorf("server log = %q, want it to contain the raw PDP error text %q so operators can still debug it",
				logBuf.String(), rawErr)
		}
	})

	t.Run("denied permission wraps ErrForbidden", func(t *testing.T) {
		pdp := &mockPDP{profile: denyAllProfile()}
		_, err := filterCallTool(ctxWithSubject(context.Background()), next, methodCallTool, req, pdp, perms, slog.Default())
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("err = %v, want it to wrap ErrForbidden", err)
		}
	})
}

// callToolRequest builds a tools/call request for a tool with raw JSON arguments.
func callToolRequest(name, argsJSON string) *mcp.ServerRequest[*mcp.CallToolParamsRaw] {
	return &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: &mcp.CallToolParamsRaw{Name: name, Arguments: json.RawMessage(argsJSON)},
	}
}

// TestCallToolScope verifies how the PDP scope is derived from tool-call
// arguments. A component or resource path without its project segment cannot
// match any project- or component-scoped policy, so it is left out and the
// scope falls back to the namespace.
func TestCallToolScope(t *testing.T) {
	tests := []struct {
		name string
		args string
		want authzcore.ResourceHierarchy
	}{
		{
			name: "no arguments",
			args: ``,
			want: authzcore.ResourceHierarchy{},
		},
		{
			name: "namespace only",
			args: `{"namespace_name":"ns1"}`,
			want: authzcore.ResourceHierarchy{Namespace: "ns1"},
		},
		{
			name: "namespace and project",
			args: `{"namespace_name":"ns1","project_name":"p1"}`,
			want: authzcore.ResourceHierarchy{Namespace: "ns1", Project: "p1"},
		},
		{
			name: "full component hierarchy is kept",
			args: `{"namespace_name":"ns1","project_name":"p1","component_name":"c1"}`,
			want: authzcore.ResourceHierarchy{Namespace: "ns1", Project: "p1", Component: "c1"},
		},
		{
			name: "full resource hierarchy is kept",
			args: `{"namespace_name":"ns1","project_name":"p1","resource_name":"r1"}`,
			want: authzcore.ResourceHierarchy{Namespace: "ns1", Project: "p1", Resource: "r1"},
		},
		{
			name: "component without project falls back to the namespace",
			args: `{"namespace_name":"ns1","component_name":"c1"}`,
			want: authzcore.ResourceHierarchy{Namespace: "ns1"},
		},
		{
			name: "resource without project falls back to the namespace",
			args: `{"namespace_name":"ns1","resource_name":"r1"}`,
			want: authzcore.ResourceHierarchy{Namespace: "ns1"},
		},
		{
			name: "invalid JSON yields the empty scope",
			args: `{`,
			want: authzcore.ResourceHierarchy{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var raw json.RawMessage
			if tt.args != "" {
				raw = json.RawMessage(tt.args)
			}
			req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
				Params: &mcp.CallToolParamsRaw{Name: "any_tool", Arguments: raw},
			}
			if got := callToolScope(req); got != tt.want {
				t.Errorf("callToolScope() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestFilterCallTool_ProjectScopedBinding runs filterCallTool against the real
// Casbin PDP with a group that holds releasebinding:view and resourcerelease:view
// only on project p1 of namespace ns1 (an AuthzRoleBinding with scope.project=p1).
// The service layer checks the full hierarchy, so the MCP gate must not refuse a
// call just because the tool has no project_name argument.
func TestFilterCallTool_ProjectScopedBinding(t *testing.T) {
	pdp, err := authzcasbin.NewEnforcer(context.Background(), authzcasbin.CasbinConfig{
		K8sClient: fake.NewClientBuilder().Build(),
	}, slog.Default())
	if err != nil {
		t.Fatalf("NewEnforcer: %v", err)
	}
	enforcer := pdp.GetEnforcer()
	if _, err := enforcer.AddGroupingPolicies([][]string{
		{"viewer", authzcore.ActionViewReleaseBinding, "ns1"},
		{"viewer", authzcore.ActionViewResourceRelease, "ns1"},
	}); err != nil {
		t.Fatalf("add role: %v", err)
	}
	if _, err := enforcer.AddPolicies([][]string{
		// Policy path of an AuthzRoleBinding with scope: {project: p1} in ns1.
		{"groups:devs", "ns/ns1/project/p1", "viewer", "ns1", "allow", "{}", "p1-viewers"},
	}); err != nil {
		t.Fatalf("add binding: %v", err)
	}

	perms := map[string]ToolPermission{
		"list_release_bindings":  {ToolName: "list_release_bindings", Action: authzcore.ActionViewReleaseBinding},
		"list_resource_releases": {ToolName: "list_resource_releases", Action: authzcore.ActionViewResourceRelease},
		"delete_component":       {ToolName: "delete_component", Action: authzcore.ActionDeleteComponent},
	}

	tests := []struct {
		name      string
		tool      string
		args      string
		wantAllow bool
	}{
		{
			name:      "component tool without project_name is allowed",
			tool:      "list_release_bindings",
			args:      `{"namespace_name":"ns1","component_name":"c1"}`,
			wantAllow: true,
		},
		{
			name:      "resource tool without project_name is allowed",
			tool:      "list_resource_releases",
			args:      `{"namespace_name":"ns1","resource_name":"r1"}`,
			wantAllow: true,
		},
		{
			name:      "component tool with the bound project keeps the full scope and is allowed",
			tool:      "list_release_bindings",
			args:      `{"namespace_name":"ns1","project_name":"p1","component_name":"c1"}`,
			wantAllow: true,
		},
		{
			name:      "component tool with another project keeps the full scope and is denied",
			tool:      "list_release_bindings",
			args:      `{"namespace_name":"ns1","project_name":"p2","component_name":"c1"}`,
			wantAllow: false,
		},
		{
			name:      "component tool in another namespace is denied",
			tool:      "list_release_bindings",
			args:      `{"namespace_name":"ns2","component_name":"c1"}`,
			wantAllow: false,
		},
		{
			name:      "resource tool in another namespace is denied",
			tool:      "list_resource_releases",
			args:      `{"namespace_name":"ns2","resource_name":"r1"}`,
			wantAllow: false,
		},
		{
			name:      "action the role does not grant is denied",
			tool:      "delete_component",
			args:      `{"namespace_name":"ns1","component_name":"c1"}`,
			wantAllow: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextCalled := false
			next := func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
				nextCalled = true
				return &mcp.CallToolResult{}, nil
			}
			_, err := filterCallTool(ctxWithSubject(context.Background()), next, methodCallTool,
				callToolRequest(tt.tool, tt.args), pdp, perms, slog.Default())
			if tt.wantAllow {
				if err != nil {
					t.Fatalf("filterCallTool() error = %v, want the call to be allowed", err)
				}
				if !nextCalled {
					t.Error("next handler was not called for an allowed call")
				}
				return
			}
			if !errors.Is(err, ErrForbidden) {
				t.Fatalf("filterCallTool() error = %v, want it to wrap ErrForbidden", err)
			}
			if nextCalled {
				t.Error("next handler must not be called for a denied call")
			}
		})
	}
}
