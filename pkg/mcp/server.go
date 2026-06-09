// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// HTTP query parameter names recognized by the MCP HTTP handler. Both are
// optional and read once per session-creation request.
const (
	// QueryParamToolsets narrows the toolsets visible via tools/list to a
	// comma-separated subset (e.g. ?toolsets=namespace,component,pe). Unknown
	// or disabled toolset names are silently ignored. When the param is
	// absent or empty, all registered toolsets are returned.
	QueryParamToolsets = "toolsets"
	// QueryParamFilterByAuthz controls whether MCP-layer authz filtering is
	// applied to tools/list and tools/call (e.g. ?filterByAuthz=false).
	// Defaults to true. The service layer enforces authz independently
	// regardless of this flag.
	QueryParamFilterByAuthz = "filterByAuthz"
	// QueryParamIncludeDeprecatedTools controls whether tools/list includes the
	// deprecated cluster-prefixed compatibility-alias tools (e.g.
	// ?includeDeprecatedTools=false). Defaults to true in v1.1: the aliases are
	// listed with a description-level deprecation banner and a structured _meta
	// marker so existing clients see a migration signal before they disappear.
	// Set this to false to preview the v1.2 surface, where the aliases are
	// hidden from tools/list but remain callable; in v1.3 they are removed.
	QueryParamIncludeDeprecatedTools = "includeDeprecatedTools"
)

// NewHTTPServer creates an MCP HTTP handler backed by a single shared server.
//
// All configured toolsets are registered up front. Per-session narrowing
// happens via query parameters parsed from the initialize request:
//   - ?toolsets=ns1,ns2              — only show tools from those toolsets in tools/list
//   - ?filterByAuthz=false           — disable MCP-layer authz filtering for the session
//   - ?includeDeprecatedTools=false  — hide the deprecated cluster-prefixed alias tools
//     (they are listed by default in v1.1 with a deprecation banner, hidden in v1.2,
//     and removed in v1.3)
//
// When pdp is non-nil the server installs a receiving middleware that filters
// tools/list results and guards tools/call invocations based on the
// authenticated user's permissions derived from their JWT token. When pdp is
// nil (authz disabled) all registered tools are visible and callable — the
// service layer still enforces authz independently. The toolset filter is
// always applied when the client requests it, regardless of pdp.
func NewHTTPServer(toolsets *tools.Toolsets, pdp authzcore.PDP) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "openchoreo-api",
		Version: "1.0.0",
	}, nil)
	perms, toolToToolsets := toolsets.Register(server)
	server.AddReceivingMiddleware(tools.NewToolFilterMiddleware(pdp, perms, toolToToolsets))
	streamable := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)
	return withSessionQueryParams(streamable)
}

// NewSTDIO creates an MCP server for STDIO transport (local CLI usage).
// Permission filtering is intentionally skipped for STDIO: there is no
// HTTP request, no JWT token, and no authenticated user identity available.
// The service layer still enforces authz for any operations that require it.
func NewSTDIO(toolsets *tools.Toolsets) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "openchoreo-cli",
		Version: "1.0.0",
	}, nil)
	toolsets.Register(server)
	return server
}

// withSessionQueryParams returns an http.Handler that extracts the optional
// MCP session-scoping query parameters (toolsets, filterByAuthz,
// includeDeprecatedTools) from the request URL and stores them on the request
// context. The MCP SDK propagates
// the initialize request's context into the long-lived session, so the values
// set here become the per-session scope used by the tool-filter middleware.
//
// Subsequent requests in a stateful session do not re-read these params — the
// session is bound to the values supplied at session creation. In stateless
// mode the params are honored on every request because each request creates a
// fresh session.
func withSessionQueryParams(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		ctx := r.Context()

		if raw := q.Get(QueryParamToolsets); raw != "" {
			if requested := parseRequestedToolsets(raw); len(requested) > 0 {
				ctx = tools.WithRequestedToolsets(ctx, requested)
			}
		}

		if raw := q.Get(QueryParamFilterByAuthz); raw != "" {
			if v, err := strconv.ParseBool(raw); err == nil {
				ctx = tools.WithFilterByAuthz(ctx, v)
			}
		}

		if raw := q.Get(QueryParamIncludeDeprecatedTools); raw != "" {
			if v, err := strconv.ParseBool(raw); err == nil {
				ctx = tools.WithIncludeDeprecatedTools(ctx, v)
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// parseRequestedToolsets parses a comma-separated list of toolset names into a
// set. Empty entries (from `,,` or trailing commas) are skipped. Unknown
// toolset names are kept in the set as-is — the filter middleware silently
// ignores them when none of the registered tools belong to that toolset, so an
// unknown name simply matches nothing.
func parseRequestedToolsets(raw string) map[tools.ToolsetType]bool {
	out := map[tools.ToolsetType]bool{}
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		out[tools.ToolsetType(name)] = true
	}
	return out
}
