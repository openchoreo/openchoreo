// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSetCacheable(t *testing.T) {
	tests := []struct {
		name      string
		req       mcp.Request
		wantTTLMs int
	}{
		{"tools/list", &mcp.ListToolsRequest{}, int(ToolsListTTL.Milliseconds())},
		{"server/discover", &mcp.ServerRequest[*mcp.DiscoverParams]{}, 0},
		{"prompts/list", &mcp.ListPromptsRequest{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mcp.Cacheable{CacheScope: "public"}
			setCacheable(context.Background(), tt.req, &c)
			if c.CacheScope != CacheScopePrivate || c.TTLMs != tt.wantTTLMs {
				t.Errorf("cacheScope/ttlMs = %q/%d, want %q/%d", c.CacheScope, c.TTLMs, CacheScopePrivate, tt.wantTTLMs)
			}
		})
	}
}

// TestNewStreamableHTTPHandlerCancelsOnDisconnect verifies that a tool
// handler's context ends when the client abandons a 2026-07-28 request.
func TestNewStreamableHTTPHandlerCancelsOnDisconnect(t *testing.T) {
	server := NewServer("test")
	started := make(chan struct{})
	cancelled := make(chan struct{})
	mcp.AddTool(server, &mcp.Tool{Name: "block"},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			close(started)
			select {
			case <-ctx.Done():
				close(cancelled)
			case <-time.After(10 * time.Second):
			}
			return &mcp.CallToolResult{}, nil, nil
		})
	srv := httptest.NewServer(NewStreamableHTTPHandler(server))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"block","arguments":{},` +
		`"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
		`"io.modelcontextprotocol/clientInfo":{"name":"test","version":"1"},` +
		`"io.modelcontextprotocol/clientCapabilities":{}}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2026-07-28")
	req.Header.Set("Mcp-Method", "tools/call")
	req.Header.Set("Mcp-Name", "block")
	go func() {
		resp, err := srv.Client().Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("tool handler did not start")
	}
	cancel()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("tool handler context was not cancelled after the client disconnected")
	}
}
