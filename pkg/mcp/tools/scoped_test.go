// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// setupScopedTestServer registers all toolsets on a server that has the tool
// filter middleware installed, then returns a connected client session and the
// shared mock handler. ctx is propagated to the session (so per-session flags such
// as WithIncludeDeprecatedTools take effect).
func setupScopedTestServer(
	t *testing.T, ctx context.Context, pdp authzcore.PDP,
) (*mcp.ClientSession, *MockCoreToolsetHandler) {
	t.Helper()
	mockHandler := NewMockCoreToolsetHandler()
	toolsets := &Toolsets{
		NamespaceToolset:  mockHandler,
		ProjectToolset:    mockHandler,
		ComponentToolset:  mockHandler,
		DeploymentToolset: mockHandler,
		BuildToolset:      mockHandler,
		PEToolset:         mockHandler,
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "scoped-test"}, nil)
	perms, toolToToolsets := toolsets.Register(server)
	server.AddReceivingMiddleware(NewToolFilterMiddleware(pdp, perms, toolToToolsets))

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "scoped-test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { clientSession.Close() })
	return clientSession, mockHandler
}

func TestToolPermissionScopedActions(t *testing.T) {
	p := ToolPermission{ToolName: "get_component_type", ScopedActions: map[string]string{
		ScopeNamespace: authzcore.ActionViewComponentType,
		ScopeCluster:   authzcore.ActionViewClusterComponentType,
	}}
	if got := p.ActionForScope(""); got != authzcore.ActionViewComponentType {
		t.Errorf("ActionForScope(\"\") = %q, want namespace action %q", got, authzcore.ActionViewComponentType)
	}
	if got := p.ActionForScope(ScopeNamespace); got != authzcore.ActionViewComponentType {
		t.Errorf("ActionForScope(namespace) = %q, want %q", got, authzcore.ActionViewComponentType)
	}
	if got := p.ActionForScope(ScopeCluster); got != authzcore.ActionViewClusterComponentType {
		t.Errorf("ActionForScope(cluster) = %q, want %q", got, authzcore.ActionViewClusterComponentType)
	}
	if got := p.ActionForScope("bogus"); got != "" {
		t.Errorf("ActionForScope(bogus) = %q, want empty", got)
	}
	actions := p.Actions()
	if len(actions) != 2 {
		t.Fatalf("Actions() = %v, want 2 entries", actions)
	}
	seen := map[string]bool{}
	for _, a := range actions {
		seen[a] = true
	}
	if !seen[authzcore.ActionViewComponentType] || !seen[authzcore.ActionViewClusterComponentType] {
		t.Errorf("Actions() = %v, missing one of the scoped actions", actions)
	}

	plain := ToolPermission{ToolName: "list_namespaces", Action: authzcore.ActionViewNamespace}
	if got := plain.ActionForScope(ScopeCluster); got != authzcore.ActionViewNamespace {
		t.Errorf("plain ActionForScope ignores scope: got %q, want %q", got, authzcore.ActionViewNamespace)
	}
	if got := plain.Actions(); len(got) != 1 || got[0] != authzcore.ActionViewNamespace {
		t.Errorf("plain Actions() = %v, want [%q]", got, authzcore.ActionViewNamespace)
	}
}

func TestScopedToolDispatch(t *testing.T) {
	cs, mock := setupScopedTestServer(t, context.Background(), nil)
	ctx := context.Background()

	// Default scope → namespace handler.
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_component_types",
		Arguments: map[string]any{"namespace_name": testNamespaceName},
	}); err != nil {
		t.Fatalf("list_component_types (default scope): %v", err)
	}
	if len(mock.calls["ListComponentTypes"]) != 1 {
		t.Errorf("expected ListComponentTypes to be called once, got %d", len(mock.calls["ListComponentTypes"]))
	}
	if len(mock.calls["ListClusterComponentTypes"]) != 0 {
		t.Errorf("expected ListClusterComponentTypes not to be called, got %d", len(mock.calls["ListClusterComponentTypes"]))
	}

	// scope=cluster → cluster handler.
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_component_types",
		Arguments: map[string]any{"scope": "cluster"},
	}); err != nil {
		t.Fatalf("list_component_types (scope=cluster): %v", err)
	}
	if len(mock.calls["ListClusterComponentTypes"]) != 1 {
		t.Errorf("expected ListClusterComponentTypes to be called once, got %d", len(mock.calls["ListClusterComponentTypes"]))
	}

	// scope=cluster on a single-resource tool routes the resource name to the cluster handler.
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_component_type",
		Arguments: map[string]any{"scope": "cluster", "name": "platform-web-app"},
	}); err != nil {
		t.Fatalf("get_component_type (scope=cluster): %v", err)
	}
	if calls := mock.calls["GetClusterComponentType"]; len(calls) != 1 {
		t.Fatalf("expected GetClusterComponentType called once, got %d", len(calls))
	} else if got := calls[0].([]interface{})[0]; got != "platform-web-app" {
		t.Errorf("GetClusterComponentType arg = %v, want platform-web-app", got)
	}
}

func TestScopedToolNamespaceRequiredForNamespaceScope(t *testing.T) {
	cs, mock := setupScopedTestServer(t, context.Background(), nil)
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_component_types",
		Arguments: map[string]any{}, // no scope, no namespace_name
	})
	if err != nil {
		t.Fatalf("CallTool returned protocol error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected a tool error result when namespace_name is missing for namespace scope")
	}
	if text := firstText(res); !strings.Contains(text, "namespace_name is required") {
		t.Errorf("error text = %q, want it to mention namespace_name is required", text)
	}
	if len(mock.calls["ListComponentTypes"]) != 0 {
		t.Errorf("handler should not be invoked when validation fails")
	}
}

func TestScopedToolInvalidScope(t *testing.T) {
	cs, _ := setupScopedTestServer(t, context.Background(), nil)
	// The `scope` argument is constrained by an enum in the input schema, so an
	// out-of-range value is rejected at the protocol layer before the handler runs.
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_component_types",
		Arguments: map[string]any{"scope": "galaxy"},
	}); err == nil {
		t.Errorf("expected an error for an invalid scope value")
	}
}

func TestDeprecatedAliasVisibleByDefault(t *testing.T) {
	// Default session in v1.1: every deprecated alias appears in tools/list with
	// a "[DEPRECATED ...]" description banner and the structured _meta marker,
	// so existing pinned-name callers see a migration signal before the surface
	// changes in v1.2/v1.3.
	cs, _ := setupScopedTestServer(t, context.Background(), nil)
	result, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := toolNameSet(result.Tools)
	byName := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		byName[tool.Name] = tool
	}
	for alias := range deprecatedToolNames {
		if !names[alias] {
			t.Errorf("deprecated alias %q should be visible in the default tools/list", alias)
			continue
		}
		tool := byName[alias]
		if !strings.HasPrefix(tool.Description, "[DEPRECATED — use ") {
			t.Errorf("deprecated alias %q description should start with [DEPRECATED ...], got %q",
				alias, tool.Description)
		}
		if !strings.Contains(tool.Description, "hidden in "+deprecatedHiddenInVersion) ||
			!strings.Contains(tool.Description, "removed in "+deprecatedRemovedInVersion) {
			t.Errorf("deprecated alias %q description should mention hidden/removed versions, got %q",
				alias, tool.Description)
		}
		meta := tool.GetMeta()
		if v, _ := meta[deprecatedMetaPrefix+"deprecated"].(bool); !v {
			t.Errorf("deprecated alias %q should carry _meta[%s]=true, got %+v",
				alias, deprecatedMetaPrefix+"deprecated", meta)
		}
		if v, _ := meta[deprecatedMetaPrefix+"hidden_in"].(string); v != deprecatedHiddenInVersion {
			t.Errorf("deprecated alias %q _meta hidden_in = %q, want %q",
				alias, v, deprecatedHiddenInVersion)
		}
		if v, _ := meta[deprecatedMetaPrefix+"removed_in"].(string); v != deprecatedRemovedInVersion {
			t.Errorf("deprecated alias %q _meta removed_in = %q, want %q",
				alias, v, deprecatedRemovedInVersion)
		}
	}
	if !names["get_component_type"] || !names["list_component_types"] {
		t.Errorf("expected canonical scope-collapsed tools to be present in tools/list")
	}
}

func TestDeprecatedAliasHiddenWhenExcluded(t *testing.T) {
	// includeDeprecatedTools=false previews the v1.2 surface: aliases drop out of
	// tools/list, while canonical scope-collapsed tools remain.
	ctx := WithIncludeDeprecatedTools(context.Background(), false)
	cs, _ := setupScopedTestServer(t, ctx, nil)
	result, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := toolNameSet(result.Tools)
	for alias := range deprecatedToolNames {
		if names[alias] {
			t.Errorf("deprecated alias %q should be hidden when includeDeprecatedTools=false", alias)
		}
	}
	if !names["get_component_type"] || !names["list_component_types"] {
		t.Errorf("canonical scope-collapsed tools should remain in tools/list")
	}
}

func TestDeprecatedAliasRoutesAndWarns(t *testing.T) {
	cs, mock := setupScopedTestServer(t, context.Background(), nil)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_cluster_component_type",
		Arguments: map[string]any{"name": "platform-web-app"},
	})
	if err != nil {
		t.Fatalf("CallTool get_cluster_component_type: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %q", firstText(res))
	}
	if calls := mock.calls["GetClusterComponentType"]; len(calls) != 1 {
		t.Fatalf("expected GetClusterComponentType called once, got %d", len(calls))
	} else if got := calls[0].([]interface{})[0]; got != "platform-web-app" {
		t.Errorf("GetClusterComponentType arg = %v, want platform-web-app", got)
	}
	// The deprecation warning is surfaced as a leading text content block.
	var warned bool
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok && strings.Contains(tc.Text, "deprecation_warning") &&
			strings.Contains(tc.Text, "get_component_type") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected a deprecation_warning content block pointing at get_component_type, got %+v", res.Content)
	}
}

func TestScopedToolAuthzFiltering(t *testing.T) {
	// A user holding only the cluster-scoped view action still sees the
	// scope-collapsed list/get tools in tools/list.
	clusterOnly := &mockPDP{profile: allowAllProfile(
		authzcore.ActionViewClusterComponentType,
		authzcore.ActionViewClusterDataPlane,
	)}
	cs, _ := setupScopedTestServer(t, ctxWithSubject(context.Background()), clusterOnly)
	result, err := cs.ListTools(ctxWithSubject(context.Background()), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := toolNameSet(result.Tools)
	if !names["list_component_types"] || !names["get_component_type"] {
		t.Errorf("user with cluster view action should see scope-collapsed component-type tools")
	}
	if !names["list_dataplanes"] {
		t.Errorf("user with cluster view action should see list_dataplanes")
	}
	// They lack the namespace component-type create action; create_component_type
	// is only shown when the user holds at least one of its scoped actions.
	if names["create_component_type"] {
		t.Errorf("create_component_type should be hidden for a user lacking any component-type create action")
	}
}

func TestScopedToolCallAuthzByScope(t *testing.T) {
	ctx := ctxWithSubject(context.Background())
	// User has namespace component-type view but NOT cluster component-type view.
	pdp := &mockPDP{profile: allowAllProfile(authzcore.ActionViewComponentType)}
	cs, mock := setupScopedTestServer(t, ctx, pdp)

	// Default (namespace) scope → allowed.
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_component_type",
		Arguments: map[string]any{"namespace_name": testNamespaceName, "name": "web-app"},
	}); err != nil {
		t.Fatalf("get_component_type (namespace scope) should be allowed: %v", err)
	}
	if len(mock.calls["GetComponentType"]) != 1 {
		t.Errorf("expected GetComponentType to have been called once, got %d", len(mock.calls["GetComponentType"]))
	}

	// scope=cluster → denied (user lacks clustercomponenttype:view).
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_component_type",
		Arguments: map[string]any{"scope": "cluster", "name": "web-app"},
	}); err == nil {
		t.Errorf("get_component_type (scope=cluster) should be denied for a user lacking clustercomponenttype:view")
	}
	if len(mock.calls["GetClusterComponentType"]) != 0 {
		t.Errorf("cluster handler must not be invoked when the call is denied")
	}
}

func TestScopedWriteToolDispatch(t *testing.T) {
	cs, mock := setupScopedTestServer(t, context.Background(), nil)
	ctx := context.Background()

	// Default scope → namespace create handler; namespace_name is threaded through.
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_component_type",
		Arguments: map[string]any{
			"namespace_name": testNamespaceName,
			"name":           "web-app",
			"display_name":   "Web App",
			"description":    "a web app component type",
			"spec":           map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("create_component_type (namespace scope): %v", err)
	}
	if res.IsError {
		t.Fatalf("create_component_type (namespace scope) returned tool error: %q", firstText(res))
	}
	if calls := mock.calls["CreateComponentType"]; len(calls) != 1 {
		t.Fatalf("expected CreateComponentType called once, got %d", len(calls))
	} else if ns := calls[0].([]interface{})[0]; ns != testNamespaceName {
		t.Errorf("CreateComponentType namespace arg = %v, want %s", ns, testNamespaceName)
	}
	if len(mock.calls["CreateClusterComponentType"]) != 0 {
		t.Errorf("cluster create handler must not run for the default scope")
	}

	// scope=cluster → cluster create handler; no namespace_name required.
	if res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_component_type",
		Arguments: map[string]any{
			"scope": "cluster",
			"name":  "platform-web-app",
			"spec":  map[string]any{},
		},
	}); err != nil {
		t.Fatalf("create_component_type (scope=cluster): %v", err)
	} else if res.IsError {
		t.Fatalf("create_component_type (scope=cluster) returned tool error: %q", firstText(res))
	}
	if len(mock.calls["CreateClusterComponentType"]) != 1 {
		t.Errorf("expected CreateClusterComponentType called once, got %d", len(mock.calls["CreateClusterComponentType"]))
	}

	// update_component_type routes by scope the same way.
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "update_component_type",
		Arguments: map[string]any{
			"scope": "cluster",
			"name":  "platform-web-app",
			"spec":  map[string]any{},
		},
	}); err != nil {
		t.Fatalf("update_component_type (scope=cluster): %v", err)
	}
	if len(mock.calls["UpdateClusterComponentType"]) != 1 {
		t.Errorf("expected UpdateClusterComponentType called once, got %d", len(mock.calls["UpdateClusterComponentType"]))
	}
}

func TestScopedWriteToolsRouteByScope(t *testing.T) {
	// Each scope-collapsed create/update tool dispatches to the namespace handler by
	// default and the cluster handler when scope=cluster.
	cases := []struct {
		tool       string
		nsMethod   string
		clMethod   string
		nsExtraArg map[string]any
	}{
		{"create_trait", "CreateTrait", "CreateClusterTrait", map[string]any{"namespace_name": testNamespaceName}},
		{"update_trait", "UpdateTrait", "UpdateClusterTrait", map[string]any{"namespace_name": testNamespaceName}},
		{"create_workflow", "CreateWorkflow", "CreateClusterWorkflow", map[string]any{"namespace_name": testNamespaceName}},
		{"update_workflow", "UpdateWorkflow", "UpdateClusterWorkflow", map[string]any{"namespace_name": testNamespaceName}},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			cs, mock := setupScopedTestServer(t, context.Background(), nil)
			ctx := context.Background()

			nsArgs := map[string]any{"name": "x", "spec": map[string]any{}}
			for k, v := range tc.nsExtraArg {
				nsArgs[k] = v
			}
			if res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tc.tool, Arguments: nsArgs}); err != nil {
				t.Fatalf("%s (namespace scope): %v", tc.tool, err)
			} else if res.IsError {
				t.Fatalf("%s (namespace scope) returned tool error: %q", tc.tool, firstText(res))
			}
			if len(mock.calls[tc.nsMethod]) != 1 {
				t.Errorf("expected %s called once, got %d", tc.nsMethod, len(mock.calls[tc.nsMethod]))
			}

			if res, err := cs.CallTool(ctx, &mcp.CallToolParams{
				Name:      tc.tool,
				Arguments: map[string]any{"scope": "cluster", "name": "x", "spec": map[string]any{}},
			}); err != nil {
				t.Fatalf("%s (scope=cluster): %v", tc.tool, err)
			} else if res.IsError {
				t.Fatalf("%s (scope=cluster) returned tool error: %q", tc.tool, firstText(res))
			}
			if len(mock.calls[tc.clMethod]) != 1 {
				t.Errorf("expected %s called once, got %d", tc.clMethod, len(mock.calls[tc.clMethod]))
			}
		})
	}
}

func TestScopedWriteToolNamespaceRequired(t *testing.T) {
	cs, mock := setupScopedTestServer(t, context.Background(), nil)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_component_type",
		Arguments: map[string]any{ // namespace scope (default) but no namespace_name
			"name": "web-app",
			"spec": map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("CallTool returned protocol error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected a tool error when namespace_name is missing for the namespace scope")
	}
	if text := firstText(res); !strings.Contains(text, "namespace_name is required") {
		t.Errorf("error text = %q, want it to mention namespace_name is required", text)
	}
	if len(mock.calls["CreateComponentType"]) != 0 {
		t.Errorf("service handler must not run when validation fails")
	}
}

func TestScopedWriteToolInvalidSpec(t *testing.T) {
	cs, mock := setupScopedTestServer(t, context.Background(), nil)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_component_type",
		Arguments: map[string]any{
			"namespace_name": testNamespaceName,
			"name":           "web-app",
			"spec":           map[string]any{"allowedTraits": "not-an-array"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool returned protocol error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected a tool error when the spec fails to decode into the typed spec")
	}
	if len(mock.calls["CreateComponentType"]) != 0 {
		t.Errorf("service handler must not run when the spec fails to decode")
	}
}

func TestScopedSchemaToolDispatch(t *testing.T) {
	cs, _ := setupScopedTestServer(t, context.Background(), nil)
	ctx := context.Background()
	for _, scope := range []string{"", "namespace", "cluster"} {
		args := map[string]any{}
		if scope != "" {
			args["scope"] = scope
		}
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{
			Name:      "get_component_type_creation_schema",
			Arguments: args,
		})
		if err != nil {
			t.Fatalf("get_component_type_creation_schema (scope=%q): %v", scope, err)
		}
		if res.IsError {
			t.Errorf("get_component_type_creation_schema (scope=%q) returned tool error: %q", scope, firstText(res))
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func toolNameSet(tools []*mcp.Tool) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, t := range tools {
		out[t.Name] = true
	}
	return out
}

func firstText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
