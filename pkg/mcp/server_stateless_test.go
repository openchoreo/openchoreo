// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	observermcp "github.com/openchoreo/openchoreo/internal/observer/mcp"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	mcpmiddleware "github.com/openchoreo/openchoreo/internal/server/middleware/mcp"
	"github.com/openchoreo/openchoreo/internal/version"
	"github.com/openchoreo/openchoreo/pkg/mcp/mcpaudit"
	"github.com/openchoreo/openchoreo/pkg/mcp/mcpserver"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// Exercise the production HTTP handlers and SDK, including context propagation.
// These requests deliberately carry no session ID or shared client state.
func statelessRPC(t *testing.T, h http.Handler, query, version, body string) map[string]json.RawMessage {
	t.Helper()
	status, response := statelessPOST(t, h, query, version, body)
	if status != http.StatusOK {
		t.Fatalf("HTTP %d: %s", status, response)
	}
	return response
}

// statelessPOST is statelessRPC without the 200 check, returning the HTTP
// status alongside the decoded JSON-RPC response.
func statelessPOST(t *testing.T, h http.Handler, query, version, body string) (int, map[string]json.RawMessage) {
	t.Helper()
	var message struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal([]byte(body), &message); err != nil {
		t.Fatal(err)
	}
	if version >= "2026-07-28" {
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
	if version >= "2026-07-28" {
		req.Header.Set("Mcp-Method", message.Method)
		if name, ok := message.Params["name"].(string); ok {
			req.Header.Set("Mcp-Name", name)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
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
		t.Fatalf("HTTP %d: decode response: %v; body=%s", w.Code, err, w.Body.String())
	}
	return w.Code, response
}

// toolsOnlyCapabilities is the only capability set our MCP servers advertise.
const toolsOnlyCapabilities = `{"tools":{}}`

// newStatelessTestServers builds the API and observer MCP handlers through
// their production constructors.
func newStatelessTestServers(t *testing.T) map[string]http.Handler {
	t.Helper()
	api := newTestMCPHandler(t, &tools.Toolsets{ProjectToolset: &fakeProjectToolset{}}, nil,
		mcpaudit.MiddlewareOptions{Emitter: newAuditTestEmitter(t, io.Discard)})
	observer, err := observermcp.NewHTTPServer(nil,
		mcpaudit.MiddlewareOptions{Emitter: newAuditTestEmitter(t, io.Discard)})
	if err != nil {
		t.Fatal(err)
	}
	return map[string]http.Handler{"api": api, "observer": observer}
}

func TestHTTPServersAdvertiseOnlyTools(t *testing.T) {
	for name, handler := range newStatelessTestServers(t) {
		t.Run(name, func(t *testing.T) {
			discover := statelessRPC(t, handler, "", "2026-07-28",
				`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`)
			var discovery struct {
				Meta         map[string]json.RawMessage `json:"_meta"`
				Capabilities json.RawMessage            `json:"capabilities"`
				CacheScope   string                     `json:"cacheScope"`
			}
			if err := json.Unmarshal(discover["result"], &discovery); err != nil {
				t.Fatal(err)
			}
			if string(discovery.Capabilities) != toolsOnlyCapabilities {
				t.Errorf("capabilities = %s, want %s", discovery.Capabilities, toolsOnlyCapabilities)
			}
			if discovery.CacheScope != mcpserver.CacheScopePrivate {
				t.Errorf("cacheScope = %q, want private", discovery.CacheScope)
			}
			var serverInfo struct {
				Version string `json:"version"`
			}
			if err := json.Unmarshal(discovery.Meta["io.modelcontextprotocol/serverInfo"], &serverInfo); err != nil {
				t.Fatalf("serverInfo: %v; result=%s", err, discover["result"])
			}
			if serverInfo.Version != version.Get().Version {
				t.Errorf("serverInfo.version = %q, want the build version %q", serverInfo.Version, version.Get().Version)
			}

			initialize := statelessRPC(t, handler, "", "2025-06-18",
				`{"jsonrpc":"2.0","id":2,"method":"initialize","params":`+
					`{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"legacy","version":"1"}}}`)
			var initialized struct {
				Capabilities json.RawMessage `json:"capabilities"`
				ServerInfo   struct {
					Version string `json:"version"`
				} `json:"serverInfo"`
			}
			if err := json.Unmarshal(initialize["result"], &initialized); err != nil {
				t.Fatal(err)
			}
			if string(initialized.Capabilities) != toolsOnlyCapabilities {
				t.Errorf("initialize capabilities = %s, want %s", initialized.Capabilities, toolsOnlyCapabilities)
			}
			if initialized.ServerInfo.Version != version.Get().Version {
				t.Errorf("initialize serverInfo.version = %q, want %q",
					initialized.ServerInfo.Version, version.Get().Version)
			}
		})
	}
}

func TestHTTPServersToolsListIsPrivatelyCacheable(t *testing.T) {
	for name, handler := range newStatelessTestServers(t) {
		t.Run(name, func(t *testing.T) {
			response := statelessRPC(t, handler, "?filterByAuthz=false", "2026-07-28",
				`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
			var result struct {
				TTLMs      int    `json:"ttlMs"`
				CacheScope string `json:"cacheScope"`
			}
			if err := json.Unmarshal(response["result"], &result); err != nil {
				t.Fatal(err)
			}
			if result.CacheScope != mcpserver.CacheScopePrivate {
				t.Errorf("cacheScope = %q, want private", result.CacheScope)
			}
			if want := int(mcpserver.ToolsListTTL.Milliseconds()); result.TTLMs != want {
				t.Errorf("ttlMs = %d, want %d", result.TTLMs, want)
			}
		})
	}
}

func TestHTTPServersRejectUnsupportedProtocolVersion(t *testing.T) {
	const unsupported = "2099-01-01"
	for name, handler := range newStatelessTestServers(t) {
		t.Run(name, func(t *testing.T) {
			status, response := statelessPOST(t, handler, "", unsupported,
				`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
			if status != http.StatusBadRequest {
				t.Errorf("HTTP %d, want 400", status)
			}
			var rpcErr struct {
				Code int                                   `json:"code"`
				Data mcpsdk.UnsupportedProtocolVersionData `json:"data"`
			}
			if err := json.Unmarshal(response["error"], &rpcErr); err != nil {
				t.Fatalf("error: %v; response=%s", err, response)
			}
			if rpcErr.Code != mcpsdk.CodeUnsupportedProtocolVersion {
				t.Errorf("code = %d, want %d", rpcErr.Code, mcpsdk.CodeUnsupportedProtocolVersion)
			}
			if rpcErr.Data.Requested != unsupported || !slices.Contains(rpcErr.Data.Supported, "2026-07-28") {
				t.Errorf("data = %+v, want requested %q and 2026-07-28 among supported", rpcErr.Data, unsupported)
			}
		})
	}
}

func TestNewSTDIOProtocolSettings(t *testing.T) {
	server := NewSTDIO(&tools.Toolsets{ProjectToolset: &fakeProjectToolset{}})
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	initialized := session.InitializeResult()
	caps, err := json.Marshal(initialized.Capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if string(caps) != toolsOnlyCapabilities {
		t.Errorf("capabilities = %s, want %s", caps, toolsOnlyCapabilities)
	}
	if initialized.ServerInfo == nil || initialized.ServerInfo.Version != version.Get().Version {
		t.Errorf("serverInfo = %+v, want the build version %q", initialized.ServerInfo, version.Get().Version)
	}

	list, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if want := int(mcpserver.ToolsListTTL.Milliseconds()); list.CacheScope != mcpserver.CacheScopePrivate ||
		list.TTLMs != want {
		t.Errorf("cacheScope/ttlMs = %q/%d, want %q/%d", list.CacheScope, list.TTLMs, mcpserver.CacheScopePrivate, want)
	}
}

func TestHTTPServersStatelessProtocols(t *testing.T) {
	for name, handler := range newStatelessTestServers(t) {
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

func TestHTTPServersListenAcknowledgedThroughAuth401(t *testing.T) {
	for name, handler := range newStatelessTestServers(t) {
		t.Run(name, func(t *testing.T) {
			auth401 := mcpmiddleware.Auth401Interceptor("http://localhost/.well-known/oauth-protected-resource", nil)
			srv := httptest.NewServer(auth401(handler))
			defer srv.Close()

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			body := `{"jsonrpc":"2.0","id":"listen:0","method":"subscriptions/listen","params":{` +
				`"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
				`"io.modelcontextprotocol/clientInfo":{"name":"test","version":"1"},` +
				`"io.modelcontextprotocol/clientCapabilities":{}},` +
				`"notifications":{"toolsListChanged":true}}}`
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("MCP-Protocol-Version", "2026-07-28")
			req.Header.Set("Mcp-Method", "subscriptions/listen")

			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("listen stream not flushed: %v", err)
			}
			defer resp.Body.Close()
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				data, ok := strings.CutPrefix(scanner.Text(), "data: ")
				if !ok {
					continue
				}
				var message struct {
					Method string `json:"method"`
				}
				if err := json.Unmarshal([]byte(data), &message); err != nil {
					t.Fatal(err)
				}
				if message.Method != "notifications/subscriptions/acknowledged" {
					t.Fatalf("first message = %q, want acknowledgement", message.Method)
				}
				return
			}
			t.Fatalf("listen stream ended without acknowledgement: %v", scanner.Err())
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
				query                string
				project, environment bool
			}{
				{"?filterByAuthz=false&toolsets=project", true, false},
				{"?filterByAuthz=false&toolsets=pe", false, true},
				{"?filterByAuthz=false", true, true},
				{"?filterByAuthz=true", false, false},
				{"", false, false},
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
				var project, environment bool
				for _, tool := range result.Tools {
					project = project || tool.Name == "create_project"
					environment = environment || tool.Name == "create_environment"
				}
				if project != tc.project || environment != tc.environment {
					t.Fatalf("%s: project/environment = %v/%v, want %v/%v",
						tc.query, project, environment, tc.project, tc.environment)
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
