// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	observermcp "github.com/openchoreo/openchoreo/internal/observer/mcp"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	"github.com/openchoreo/openchoreo/pkg/mcp/mcpaudit"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// Exercise the production HTTP handlers and SDK, including context propagation.
// These requests deliberately carry no session ID or shared client state.
func statelessRPC(t *testing.T, h http.Handler, query, version, body string) map[string]json.RawMessage {
	t.Helper()
	var message struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal([]byte(body), &message); err != nil {
		t.Fatal(err)
	}
	if version == "2026-07-28" {
		message.Params["_meta"] = map[string]any{
			"io.modelcontextprotocol/protocolVersion":    version,
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
			"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "test", "version": "1"},
		}
		encoded, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": message.Method, "params": message.Params,
		})
		if err != nil {
			t.Fatal(err)
		}
		body = string(encoded)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp"+query, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", version)
	if version == "2026-07-28" {
		req.Header.Set("Mcp-Method", message.Method)
		if name, ok := message.Params["name"].(string); ok {
			req.Header.Set("Mcp-Name", name)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	if id := w.Header().Get("Mcp-Session-Id"); id != "" {
		t.Fatalf("unexpected session ID: %q", id)
	}
	payload := w.Body.String()
	if strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream") {
		payload = ""
		for line := range strings.SplitSeq(w.Body.String(), "\n") {
			if strings.HasPrefix(line, "data: ") {
				payload = strings.TrimPrefix(line, "data: ")
			}
		}
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	return response
}

func TestHTTPServersStatelessProtocols(t *testing.T) {
	api := newTestMCPHandler(t, &tools.Toolsets{ProjectToolset: &fakeProjectToolset{}}, nil,
		mcpaudit.MiddlewareOptions{Emitter: newAuditTestEmitter(t, io.Discard)})
	for name, handler := range map[string]http.Handler{
		"api":      api,
		"observer": observermcp.NewHTTPServer(nil),
	} {
		t.Run(name, func(t *testing.T) {
			discover := statelessRPC(t, handler, "", "2026-07-28",
				`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`)
			var discovery struct {
				SupportedVersions []string `json:"supportedVersions"`
			}
			if err := json.Unmarshal(discover["result"], &discovery); err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(discovery.SupportedVersions, "2026-07-28") {
				t.Fatalf("new protocol not advertised: %s", discover)
			}
			for _, version := range []string{"2025-03-26", "2025-06-18", "2025-11-25", "2026-07-28"} {
				t.Run(version, func(t *testing.T) {
					// Listing must work without first initializing a server-side session.
					response := statelessRPC(t, handler, "", version,
						`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
					if response["error"] != nil || response["result"] == nil {
						t.Fatalf("tools/list failed: %s", response)
					}
				})
			}
			// Legacy clients still initialize and receive their negotiated version.
			response := statelessRPC(t, handler, "", "2025-06-18",
				`{"jsonrpc":"2.0","id":2,"method":"initialize","params":`+
					`{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"legacy","version":"1"}}}`)
			var result struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			if err := json.Unmarshal(response["result"], &result); err != nil {
				t.Fatal(err)
			}
			if result.ProtocolVersion != "2025-06-18" {
				t.Fatalf("negotiated version = %q", result.ProtocolVersion)
			}
		})
	}
}

type statelessToolset struct{ tools.AllToolsetsHandler }

func TestHTTPServerFiltersEveryRequest(t *testing.T) {
	all := tools.NewToolsets(&statelessToolset{}, map[tools.ToolsetType]bool{
		tools.ToolsetProject: true, tools.ToolsetPE: true,
	})
	handler := newTestMCPHandler(t, all, &fakeAuditPDP{profile: denyAllAuditProfile()},
		mcpaudit.MiddlewareOptions{Emitter: newAuditTestEmitter(t, io.Discard)})
	for _, version := range []string{"2025-06-18", "2026-07-28"} {
		t.Run(version, func(t *testing.T) {
			for _, tc := range []struct {
				query                            string
				project, environment, deprecated bool
			}{
				{"?filterByAuthz=false&includeDeprecatedTools=true", true, true, true},
				{"?filterByAuthz=false&toolsets=project", true, false, false},
				{"?filterByAuthz=false&toolsets=pe", false, true, false},
				{"?filterByAuthz=false", true, true, false},
				{"?filterByAuthz=true", false, false, false},
				{"", false, false, false},
			} {
				response := statelessRPC(t, handler, tc.query, version,
					`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
				var result struct {
					Tools []struct {
						Name string `json:"name"`
					} `json:"tools"`
				}
				if response["error"] != nil {
					t.Fatalf("list: %s", response["error"])
				}
				if err := json.Unmarshal(response["result"], &result); err != nil {
					t.Fatal(err)
				}
				var project, environment, deprecated bool
				for _, tool := range result.Tools {
					project = project || tool.Name == "create_project"
					environment = environment || tool.Name == "create_environment"
					deprecated = deprecated || tools.IsDeprecatedTool(tool.Name)
				}
				if project != tc.project || environment != tc.environment || deprecated != tc.deprecated {
					t.Fatalf("%s: project/environment/deprecated = %v/%v/%v, want %v/%v/%v",
						tc.query, project, environment, deprecated, tc.project, tc.environment, tc.deprecated)
				}
			}
		})
	}
}

func TestHTTPServerCallAuthorizationEveryRequest(t *testing.T) {
	handler := newTestMCPHandler(t, &tools.Toolsets{ProjectToolset: &fakeProjectToolset{}},
		&fakeAuditPDP{profile: denyAllAuditProfile()},
		mcpaudit.MiddlewareOptions{Emitter: newAuditTestEmitter(t, io.Discard)})
	for _, query := range []string{"?filterByAuthz=false", "", "?filterByAuthz=false", "?filterByAuthz=true"} {
		response := statelessRPC(t, handler, query, "2026-07-28",
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+
				`{"name":"create_project","arguments":{"namespace_name":"test","name":"example"}}}`)
		denied := response["error"] != nil
		if denied != (query != "?filterByAuthz=false") {
			t.Fatalf("%s: unexpected authorization result: %s", query, response)
		}
		if !denied {
			var result struct {
				IsError bool `json:"isError"`
			}
			if err := json.Unmarshal(response["result"], &result); err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("tool failed: %s", response)
			}
		}
	}
}

func TestHTTPServerSubjectDoesNotPersist(t *testing.T) {
	handler, err := NewHTTPServer(slog.Default(), &tools.Toolsets{ProjectToolset: &fakeProjectToolset{}},
		&fakeAuditPDP{profile: allowAllAuditProfile(authzcore.ActionCreateProject)},
		mcpaudit.MiddlewareOptions{Emitter: newAuditTestEmitter(t, io.Discard)})
	if err != nil {
		t.Fatal(err)
	}
	for _, authenticated := range []bool{true, false, true} {
		requestHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authenticated {
				r = r.WithContext(auth.SetSubjectContext(r.Context(), &auth.SubjectContext{ID: "test-user", Type: "user"}))
			}
			handler.ServeHTTP(w, r)
		})
		response := statelessRPC(t, requestHandler, "", "2026-07-28",
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+
				`{"name":"create_project","arguments":{"namespace_name":"test","name":"example"}}}`)
		if (response["error"] == nil) != authenticated {
			t.Fatalf("authenticated=%v: %s", authenticated, response)
		}
	}
}
