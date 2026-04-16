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
	}))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms))

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
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms))

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
