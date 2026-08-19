// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcpaudit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// methodCallTool is the MCP method name for tool invocation.
const methodCallTool = "tools/call"

// newAuditMiddleware returns the mcp.Middleware that emits one audit event
// per tools/call on a bound tool. Everything else — initialize, ping,
// tools/list, notifications, and calls to unbound tools — passes through
// untouched and emits nothing; an unbound tool is expected P0 behavior (full
// coverage is P1), not a gap to patch here.
//
// bindings is read-only from here on — required because concurrent
// tools/call invocations on one MCP session run in parallel goroutines
// (verified: mcp/server.go calls jsonrpc2.Async for every tools/call).
func newAuditMiddleware(emitter *audit.Emitter, bindings map[string]audit.MCPBinding, enabled bool) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (res mcp.Result, err error) {
			if !enabled || method != methodCallTool {
				return next(ctx, method, req)
			}

			toolName := callToolName(req)
			binding, bound := bindings[toolName]
			if !bound {
				return next(ctx, method, req)
			}
			op := binding.Operation

			// Seed a placeholder resource (name only) before calling next: a
			// denial never reaches the handler that would set the real UID, so
			// this is the only source of resource.name on a denied call.
			// resource.type needs no seed here — it is stamped from
			// op.ResourceType at emit time regardless (see buildEvent).
			resourceName := extractResourceArg(req, binding.ResourceArg)
			ctx, auditData := audit.NewAuditContext(ctx, &audit.Resource{Name: resourceName})

			// A panic in next must still produce an audit record — recover just
			// long enough to emit as a failure, then re-panic. Named returns let
			// this closure see next's real res/err on the non-panic path.
			defer func() {
				if p := recover(); p != nil {
					audit.EmitFromContext(
						ctx, emitter, op, audit.OriginMCP, audit.ResultFailure, auditData, requestHeader(req), "",
					)
					panic(p)
				}
				audit.EmitFromContext(
					ctx, emitter, op, audit.OriginMCP, classifyResult(res, err), auditData, requestHeader(req), "",
				)
			}()

			res, err = next(ctx, method, req)
			return res, err
		}
	}
}

// classifyResult maps a tools/call outcome to an audit Result. Denial
// (ErrNoSubject, ErrForbidden) is distinguished from failure (ErrPDPFailure,
// any other protocol error, or a tool-execution error) so a PDP outage is
// never recorded as if the user had actually been denied by policy.
//
// This only recognizes denials raised by the MCP-layer authz filter (the
// default: every session unless it opts out via ?filterByAuthz=false — see
// server.QueryParamFilterByAuthz). A denial raised by the service layer
// itself — reachable in a filterByAuthz=false session, where the service
// layer is the only authz enforcement left — surfaces as a tool execution
// error and is recorded as "failure", not "denied": the go-sdk's tools/call
// dispatch converts a handler-returned error into CallToolResult.IsError
// before this middleware ever sees it, discarding the original error's
// identity (e.g. services.ErrForbidden) along the way. There is no local
// hook to recover it without wrapping every MCP tool handler, so this is
// documented as a known asymmetry with REST (which classifies purely by
// status code, see middleware.go's determineResult) rather than fixed.
func classifyResult(res mcp.Result, err error) audit.Result {
	switch {
	case errors.Is(err, tools.ErrNoSubject), errors.Is(err, tools.ErrForbidden):
		return audit.ResultDenied
	case err != nil:
		return audit.ResultFailure
	}
	if result, ok := res.(*mcp.CallToolResult); ok && result != nil && result.IsError {
		return audit.ResultFailure
	}
	return audit.ResultSuccess
}

// callToolName extracts the tool name from a tools/call Request. Mirrors
// pkg/mcp/tools' unexported callToolName — duplicated rather than exported
// across packages for a two-line helper.
func callToolName(req mcp.Request) string {
	if req == nil {
		return ""
	}
	if p, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok && p != nil {
		return p.Name
	}
	return ""
}

// extractResourceArg reads the named field out of a tools/call request's raw
// JSON arguments. Returns "" if the argument is absent or not a string.
func extractResourceArg(req mcp.Request, argName string) string {
	if argName == "" || req == nil {
		return ""
	}
	params, ok := req.GetParams().(*mcp.CallToolParamsRaw)
	if !ok || params == nil || len(params.Arguments) == 0 {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal(params.Arguments, &raw); err != nil {
		return ""
	}
	if v, ok := raw[argName].(string); ok {
		return v
	}
	return ""
}

// requestHeader returns the HTTP header carrying this specific tools/call —
// req.GetExtra().Header is the live header map of the POST carrying that
// call, not a session-level or cached value. Returns nil (not an error) when
// unavailable, e.g. a unit test that constructs a bare *mcp.ServerRequest.
func requestHeader(req mcp.Request) http.Header {
	if req == nil {
		return nil
	}
	extra := req.GetExtra()
	if extra == nil {
		return nil
	}
	return extra.Header
}
