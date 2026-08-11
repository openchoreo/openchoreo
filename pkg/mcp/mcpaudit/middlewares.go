// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcpaudit

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// MiddlewareOptions carries the dependencies NewMiddleware needs.
type MiddlewareOptions struct {
	// Emitter is the single *audit.Emitter shared with the REST adapter, so
	// one policy applies regardless of which surface produced the event.
	// Must not be nil.
	Emitter *audit.Emitter
	// Bindings declares which MCP tools map onto which audited operations,
	// keyed by tool name. apiaudit.MCPBindings() in production.
	Bindings map[string]audit.MCPBinding
	// Enabled mirrors config.AuditConfig.Enabled. When false, the returned
	// middleware skips all audit-related work and passes straight through,
	// exactly like an unbound tool.
	Enabled bool
}

// NewMiddleware returns the mcp.Middleware that emits one audit event per
// tools/call on a bound tool. This package only builds the middleware — the
// caller (pkg/mcp's NewHTTPServer) owns composing it into the receiving chain
// and must list it first so it wraps outermost (see that file for why).
func NewMiddleware(opts MiddlewareOptions) mcp.Middleware {
	if opts.Emitter == nil {
		panic("audit: MiddlewareOptions.Emitter must not be nil")
	}
	for tool, b := range opts.Bindings {
		if b.Operation == nil {
			panic(fmt.Sprintf("audit: MCP binding for tool %q has a nil Operation", tool))
		}
	}
	return newAuditMiddleware(opts.Emitter, opts.Bindings, opts.Enabled)
}
