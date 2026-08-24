// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	"github.com/openchoreo/openchoreo/pkg/mcp/mcpaudit"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// fakeProjectToolset implements tools.ProjectToolsetHandler by embedding the
// (nil) interface and overriding only the three methods this test exercises.
// Any other method panics if called — acceptable since this test never calls
// list/get tools.
type fakeProjectToolset struct {
	tools.ProjectToolsetHandler
}

func (f *fakeProjectToolset) CreateProject(
	_ context.Context, _ string, req *gen.CreateProjectJSONRequestBody,
) (any, error) {
	return map[string]any{"name": req.Metadata.Name}, nil
}

func (f *fakeProjectToolset) UpdateProject(
	_ context.Context, _, projectName string, _ *gen.PatchProjectRequest,
) (any, error) {
	return map[string]any{"name": projectName}, nil
}

func (f *fakeProjectToolset) DeleteProject(_ context.Context, _, projectName string) (any, error) {
	return map[string]any{"name": projectName, "action": "deleted"}, nil
}

// fakePEToolset implements tools.PEToolsetHandler by embedding the (nil)
// interface and overriding only the three environment methods this test
// exercises.
type fakePEToolset struct {
	tools.PEToolsetHandler
}

func (f *fakePEToolset) CreateEnvironment(
	_ context.Context, _ string, req *gen.CreateEnvironmentJSONRequestBody,
) (any, error) {
	return map[string]any{"name": req.Metadata.Name}, nil
}

func (f *fakePEToolset) UpdateEnvironment(
	_ context.Context, _ string, req *gen.UpdateEnvironmentJSONRequestBody,
) (any, error) {
	return map[string]any{"name": req.Metadata.Name}, nil
}

func (f *fakePEToolset) DeleteEnvironment(_ context.Context, _, envName string) (any, error) {
	return map[string]any{"name": envName, "action": "deleted"}, nil
}

// fakeCreateProjectWithUID is used by the success-path test: it calls
// audit.SetResource with a real UID, exactly as internal/openchoreo-api/mcphandlers
// does after a successful CreateProject, so the test proves the placeholder
// resource seeded before next() is genuinely overwritable by the handler.
type fakeCreateProjectWithUID struct {
	fakeProjectToolset
}

func (f *fakeCreateProjectWithUID) CreateProject(
	ctx context.Context, namespaceName string, req *gen.CreateProjectJSONRequestBody,
) (any, error) {
	created := &openchoreov1alpha1.Project{}
	created.Name = req.Metadata.Name
	created.UID = "uid-from-handler"
	audit.SetResource(ctx, &audit.Resource{Namespace: namespaceName, ID: string(created.UID), Name: created.Name})
	return map[string]any{"name": created.Name}, nil
}

// fakeAuditPDP is a minimal authzcore.PDP test double.
type fakeAuditPDP struct {
	profile *authzcore.UserCapabilitiesResponse
}

func (p *fakeAuditPDP) Evaluate(context.Context, *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	return &authzcore.Decision{Decision: true}, nil
}

func (p *fakeAuditPDP) BatchEvaluate(
	context.Context, *authzcore.BatchEvaluateRequest,
) (*authzcore.BatchEvaluateResponse, error) {
	return &authzcore.BatchEvaluateResponse{}, nil
}

func (p *fakeAuditPDP) GetSubjectProfile(
	context.Context, *authzcore.ProfileRequest,
) (*authzcore.UserCapabilitiesResponse, error) {
	return p.profile, nil
}

func allowAllAuditProfile(actions ...string) *authzcore.UserCapabilitiesResponse {
	caps := make(map[string]*authzcore.ActionCapability, len(actions))
	for _, a := range actions {
		caps[a] = &authzcore.ActionCapability{Allowed: []*authzcore.CapabilityResource{{Path: "namespace/test"}}}
	}
	return &authzcore.UserCapabilitiesResponse{Capabilities: caps}
}

func denyAllAuditProfile() *authzcore.UserCapabilitiesResponse {
	return &authzcore.UserCapabilitiesResponse{Capabilities: map[string]*authzcore.ActionCapability{}}
}

// auditBindingsForTest is a small, hermetic stand-in for
// apiaudit.MCPBindings() — deliberately not importing
// internal/openchoreo-api/audit here so this test's fixture doesn't depend on
// that package's exact contents, only on the 6 tool bindings NewHTTPServer is
// expected to wire.
func auditBindingsForTest() map[audit.MCPBindingKey]audit.MCPBinding {
	op := func(id, action, resourceType string) *audit.Operation {
		return &audit.Operation{ID: id, Action: action, ResourceType: resourceType, Category: audit.CategoryManagement}
	}
	key := func(toolName string) audit.MCPBindingKey { return audit.MCPBindingKey{ToolName: toolName} }
	return map[audit.MCPBindingKey]audit.MCPBinding{
		key("create_project"): {Operation: op("CreateProject", "create_project", "projects"), ResourceArg: "name"},
		key("update_project"): {Operation: op("UpdateProject", "update_project", "projects"), ResourceArg: "project_name"},
		key("delete_project"): {Operation: op("DeleteProject", "delete_project", "projects"), ResourceArg: "project_name"},

		key("create_environment"): {
			Operation: op("CreateEnvironment", "create_environment", "environments"), ResourceArg: "name",
		},
		key("update_environment"): {
			Operation: op("UpdateEnvironment", "update_environment", "environments"), ResourceArg: "name",
		},
		key("delete_environment"): {
			Operation: op("DeleteEnvironment", "delete_environment", "environments"), ResourceArg: "name",
		},
	}
}

// withTestSubject mimics the JWT middleware's effect on the request context,
// without pulling in real token validation — this test is only concerned with
// audit wiring, not authentication.
func withTestSubject(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.SetSubjectContext(r.Context(), &auth.SubjectContext{ID: "test-user", Type: "user"})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newAuditTestEmitter(t *testing.T, logger *slog.Logger) *audit.Emitter {
	t.Helper()
	policies, errs := audit.NewPolicySet(coreconfig.NewPath("audit"), audit.Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected policy validation errors: %v", errs)
	}
	emitter, err := audit.NewEmitter("openchoreo-api", policies, audit.NewLogger(logger))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return emitter
}

// newTestMCPHandler builds the MCP HTTP handler via the real production
// constructor (NewHTTPServer), failing loudly if auditOpts is misconfigured —
// real production callers (main.go) must check the same error.
func newTestMCPHandler(
	t *testing.T, toolsets *tools.Toolsets, pdp authzcore.PDP, auditOpts mcpaudit.MiddlewareOptions,
) http.Handler {
	t.Helper()
	server, err := NewHTTPServer(slog.Default(), toolsets, pdp, auditOpts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return withTestSubject(server)
}

// connectTestClient connects a real mcp.Client to server over the streamable
// HTTP transport (not the in-process NewInMemoryTransports helper — that path
// bypasses NewHTTPServer entirely and would prove nothing about production
// wiring). The caller must close the returned session.
func connectTestClient(t *testing.T, ctx context.Context, server *httptest.Server) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: server.Client()}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	return session
}

func auditRecordsFromLog(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	for line := range strings.SplitSeq(buf.String(), "\n") {
		if !strings.Contains(line, `"msg":"AUDIT-LOG"`) {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("failed to unmarshal audit log line %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

// TestNewHTTPServer_AuditWired guards against a repeat of #2588 on the MCP
// surface: it drives real requests, through a real mcp.Client, against the
// exact production constructor (NewHTTPServer) and asserts an AUDIT-LOG
// record is emitted. A hand-assembled middleware chain would only prove
// mcpaudit works in isolation, not that NewHTTPServer wires it.
func TestNewHTTPServer_AuditWired(t *testing.T) {
	t.Run("success emits one record with the handler's resource UID", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil))
		emitter := newAuditTestEmitter(t, logger)

		toolsets := &tools.Toolsets{
			ProjectToolset: &fakeCreateProjectWithUID{},
		}
		pdp := &fakeAuditPDP{profile: allowAllAuditProfile(authzcore.ActionCreateProject)}

		handler := newTestMCPHandler(t, toolsets, pdp, mcpaudit.MiddlewareOptions{
			Emitter: emitter, Bindings: auditBindingsForTest(), Enabled: true,
		})
		server := httptest.NewServer(handler)
		defer server.Close()

		ctx := context.Background()
		session := connectTestClient(t, ctx, server)
		defer session.Close()

		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "create_project",
			Arguments: map[string]any{"namespace_name": "test-ns", "name": "wired-project"},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if res.IsError {
			t.Fatalf("CallTool returned a tool error: %+v", res)
		}

		records := auditRecordsFromLog(t, &buf)
		if len(records) != 1 {
			t.Fatalf("expected exactly one AUDIT-LOG record, got %d:\n%s", len(records), buf.String())
		}
		record := records[0]

		if record["action"] != "create_project" {
			t.Errorf("action = %v, want create_project", record["action"])
		}
		if record["origin"] != "mcp" {
			t.Errorf("origin = %v, want mcp", record["origin"])
		}
		if record["result"] != "success" {
			t.Errorf("result = %v, want success", record["result"])
		}
		resource, ok := record["resource"].(map[string]any)
		if !ok {
			t.Fatalf("resource was not populated: %v", record)
		}
		if resource["id"] != "uid-from-handler" {
			t.Errorf("resource.id = %v, want the handler-set UID, not just the placeholder name", resource["id"])
		}
		if resource["namespace"] != "test-ns" {
			t.Errorf("resource.namespace = %v, want test-ns", resource["namespace"])
		}
	})

	t.Run("PDP denial emits result=denied, distinguishable from failure", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil))
		emitter := newAuditTestEmitter(t, logger)

		toolsets := &tools.Toolsets{
			ProjectToolset: &fakeProjectToolset{},
		}
		// Profile grants nothing — create_project is denied by the authz filter,
		// which runs INSIDE (wrapped by) the audit middleware per NewHTTPServer's
		// documented ordering. If audit stopped being outermost, this assertion
		// is what would break.
		pdp := &fakeAuditPDP{profile: denyAllAuditProfile()}

		handler := newTestMCPHandler(t, toolsets, pdp, mcpaudit.MiddlewareOptions{
			Emitter: emitter, Bindings: auditBindingsForTest(), Enabled: true,
		})
		server := httptest.NewServer(handler)
		defer server.Close()

		ctx := context.Background()
		session := connectTestClient(t, ctx, server)
		defer session.Close()

		_, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "create_project",
			Arguments: map[string]any{"namespace_name": "test-ns", "name": "denied-project"},
		})
		if err == nil {
			t.Fatal("expected CallTool to fail for a denied subject, got nil error")
		}

		records := auditRecordsFromLog(t, &buf)
		if len(records) != 1 {
			t.Fatalf("expected exactly one AUDIT-LOG record, got %d:\n%s", len(records), buf.String())
		}
		record := records[0]

		if record["result"] != "denied" {
			t.Errorf("result = %v, want denied", record["result"])
		}
		resource, ok := record["resource"].(map[string]any)
		if !ok {
			t.Fatalf("resource was not populated even on denial: %v", record)
		}
		if resource["name"] != "denied-project" {
			t.Errorf("resource.name = %v, want the placeholder name from the raw call arguments", resource["name"])
		}
		if _, hasID := resource["id"]; hasID {
			t.Errorf("resource.id = %v, want absent on a denied call (handler never ran to set a real UID)", resource["id"])
		}
		if resource["namespace"] != "test-ns" {
			t.Errorf("resource.namespace = %v, want test-ns from the placeholder seed (handler never ran)",
				resource["namespace"])
		}
	})

	t.Run("environment binding resolves independently of the project binding", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil))
		emitter := newAuditTestEmitter(t, logger)

		toolsets := &tools.Toolsets{
			PEToolset: &fakePEToolset{},
		}
		pdp := &fakeAuditPDP{profile: allowAllAuditProfile(authzcore.ActionCreateEnvironment)}

		handler := newTestMCPHandler(t, toolsets, pdp, mcpaudit.MiddlewareOptions{
			Emitter: emitter, Bindings: auditBindingsForTest(), Enabled: true,
		})
		server := httptest.NewServer(handler)
		defer server.Close()

		ctx := context.Background()
		session := connectTestClient(t, ctx, server)
		defer session.Close()

		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "create_environment",
			Arguments: map[string]any{
				"namespace_name": "test-ns", "name": "wired-env", "data_plane_ref": "dp-1",
			},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if res.IsError {
			t.Fatalf("CallTool returned a tool error: %+v", res)
		}

		records := auditRecordsFromLog(t, &buf)
		if len(records) != 1 {
			t.Fatalf("expected exactly one AUDIT-LOG record, got %d:\n%s", len(records), buf.String())
		}
		record := records[0]
		if record["action"] != "create_environment" {
			t.Errorf("action = %v, want create_environment", record["action"])
		}
		resource, ok := record["resource"].(map[string]any)
		if !ok {
			t.Fatalf("resource was not populated: %v", record)
		}
		if resource["name"] != "wired-env" {
			t.Errorf("resource.name = %v, want wired-env — confirms auditBindingsForTest's \"name\" ResourceArg for "+
				"create_environment (not \"env_name\")", resource["name"])
		}
	})
}

// TestNewHTTPServer_AuditDisabled guards against a repeat of the bug where
// audit.enabled=false silenced REST but MCP kept emitting: driven through the
// exact production constructor (NewHTTPServer) with auditEnabled=false, a
// bound tool call must still succeed but produce zero AUDIT-LOG records.
func TestNewHTTPServer_AuditDisabled(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	emitter := newAuditTestEmitter(t, logger)

	toolsets := &tools.Toolsets{
		ProjectToolset: &fakeCreateProjectWithUID{},
	}
	pdp := &fakeAuditPDP{profile: allowAllAuditProfile(authzcore.ActionCreateProject)}

	handler := newTestMCPHandler(t, toolsets, pdp, mcpaudit.MiddlewareOptions{
		Emitter: emitter, Bindings: auditBindingsForTest(), Enabled: false,
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx := context.Background()
	session := connectTestClient(t, ctx, server)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "create_project",
		Arguments: map[string]any{"namespace_name": "test-ns", "name": "disabled-project"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool returned a tool error: %+v", res)
	}

	records := auditRecordsFromLog(t, &buf)
	if len(records) != 0 {
		t.Fatalf("expected zero AUDIT-LOG records with audit disabled, got %d:\n%s", len(records), buf.String())
	}
}
