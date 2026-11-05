// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package mcpserver holds the go-sdk settings shared by every OpenChoreo MCP
// server: openchoreo-api, the observer and the STDIO server.
package mcpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/version"
)

// CacheScopePrivate marks a result as reusable only by the requesting user's
// client (SEP-2549).
const CacheScopePrivate = "private"

// ToolsListTTL is how long a client may reuse a tools/list result. Tools are
// registered once at startup, so the list changes only when the caller's
// permissions do; a newly granted tool may stay hidden for up to this long.
const ToolsListTTL = 5 * time.Minute

// NewServer returns an MCP server identifying itself as name at the build
// version.
//
// It advertises only {"tools":{}}: tools are registered once at startup, so no
// notifications/tools/list_changed is ever sent, and the logging capability
// is deprecated (SEP-2577) and unused.
func NewServer(name string) *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{
		Name:    name,
		Version: version.Get().Version,
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
		SetCacheable: setCacheable,
	})
}

// setCacheable marks every cacheable result private, since tools/list is
// narrowed per caller by authz and ?toolsets=, and lets clients reuse
// tools/list for ToolsListTTL.
func setCacheable(_ context.Context, req mcp.Request, c *mcp.Cacheable) {
	c.CacheScope = CacheScopePrivate
	if _, ok := req.(*mcp.ListToolsRequest); ok {
		c.TTLMs = int(ToolsListTTL.Milliseconds())
	}
}

// NewStreamableHTTPHandler serves server over stateless streamable HTTP.
// Under 2026-07-28 the POST is the whole request lifecycle, so a handler's
// context is cancelled when the client disconnects.
func NewStreamableHTTPHandler(server *mcp.Server) *mcp.StreamableHTTPHandler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless:                    true,
		PropagateRequestCancellation: true,
	})
}
