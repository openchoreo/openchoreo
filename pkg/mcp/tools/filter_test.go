// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// ---------------------------------------------------------------------------
// Helpers / mocks
// ---------------------------------------------------------------------------

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

// TestNewToolFilterMiddlewareNilPDP verifies that when pdp is nil all tools are
// returned unfiltered (graceful degradation when authz is disabled).
func TestNewToolFilterMiddlewareNilPDP(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "list_namespaces"}, func(
		ctx context.Context, req *mcp.CallToolRequest, args any,
	) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})

	server.AddReceivingMiddleware(NewToolFilterMiddleware(nil, map[string]ToolPermission{
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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms, nil))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms, nil))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms, nil))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms, nil))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms, nil))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms, nil))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms, nil))

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

	server.AddReceivingMiddleware(NewToolFilterMiddleware(nil, nil, toolToToolsets))

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

	server.AddReceivingMiddleware(NewToolFilterMiddleware(nil, nil, toolToToolsets))

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

	server.AddReceivingMiddleware(NewToolFilterMiddleware(nil, nil, toolToToolsets))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms, toolToToolsets))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms, nil))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms, nil))

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

	server.AddReceivingMiddleware(NewToolFilterMiddleware(nil, nil, toolToToolsets))

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
