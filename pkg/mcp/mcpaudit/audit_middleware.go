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
// untouched and emits nothing; an unbound tool is expected today (not every
// tool is bound to an audit operation yet), not a gap to patch here.
//
// bindings is read-only from here on — required because concurrent
// tools/call invocations on one MCP session run in parallel goroutines
// (verified: mcp/server.go calls jsonrpc2.Async for every tools/call).
func newAuditMiddleware(
	emitter *audit.Emitter, bindings map[audit.MCPBindingKey]audit.MCPBinding, enabled bool,
) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (res mcp.Result, err error) {
			if !enabled || method != methodCallTool {
				return next(ctx, method, req)
			}

			toolName := callToolName(req)
			args := parseCallArguments(req)
			binding, bound := resolveBinding(bindings, toolName, args)
			if !bound {
				return next(ctx, method, req)
			}
			op := binding.Operation

			// Seed a placeholder resource (namespace and name) before calling
			// next: a denial never reaches the handler that would set the
			// real UID, so this is the only source of resource.namespace/name
			// on a denied call. Every bound tool takes a namespace_name
			// argument (the same convention pkg/mcp/tools' callToolScope
			// relies on), so it needs no per-operation config. resource.type
			// needs no seed here — it is stamped from op.ResourceType at emit
			// time regardless (see buildEvent).
			resourceName := argFromParsed(args, binding.ResourceArg)
			namespaceName := argFromParsed(args, "namespace_name")
			ctx, auditData := audit.NewAuditContext(ctx, &audit.Resource{Namespace: namespaceName, Name: resourceName})

			// Seed the hierarchy from the same namespace_name/project_name/
			// component_name/resource_name convention callToolScope uses
			// (pkg/mcp/tools/filter.go), so the audit seed and the MCP authz
			// scope can't drift apart. This is a claim, not a resolved
			// decision — it's what covers a filter-layer denial, which never
			// reaches AuthzChecker.Check (and thus audit.SetHierarchy) at
			// all. A service-layer Check later overrides it with the
			// resolved hierarchy — see audit.SeedHierarchy's doc comment.
			audit.SeedHierarchy(ctx, audit.Hierarchy{
				Namespace: namespaceName,
				Project:   argFromParsed(args, "project_name"),
				Component: argFromParsed(args, "component_name"),
				Resource:  argFromParsed(args, "resource_name"),
			})

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

// classifyResult maps a tools/call outcome to an audit Result.
// ErrNoSubject (no authenticated subject) is distinguished from ErrForbidden
// (an authenticated subject the PDP refused) — see ResultUnauthenticated's
// doc comment — and both are distinguished from failure (ErrPDPFailure, any
// other protocol error, or a tool-execution error) so a PDP outage is never
// recorded as if the user had actually been denied by policy.
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
	case errors.Is(err, tools.ErrNoSubject):
		return audit.ResultUnauthenticated
	case errors.Is(err, tools.ErrForbidden):
		return audit.ResultDenied
	case err != nil:
		return audit.ResultFailure
	}
	if result, ok := res.(*mcp.CallToolResult); ok && result != nil && result.IsError {
		return audit.ResultFailure
	}
	return audit.ResultSuccess
}

// resolveBinding looks up toolName's binding, resolving scope for a
// scope-collapsed tool (one MCP tool fanning out to more than one audited
// operation, e.g. a namespace-scoped and a cluster-scoped REST operation)
// before the lookup that needs it — see audit.MCPBindingKey's doc comment.
// args is the call's arguments already parsed by parseCallArguments, shared
// with newAuditMiddleware's own resource-name lookup on the same call so the
// raw JSON is unmarshaled once, not once per argument read from it.
//
// The unscoped key (Scope: "") is tried first: it's the common case (a tool
// bound to exactly one operation), so no scope argument needs extracting for
// the vast majority of bound tools. Only a tool with no unscoped binding
// falls through to resolving scope, defaulting an absent or unrecognized
// value to tools.ScopeNamespace — matching resolveScope's own default in
// pkg/mcp/tools, so a scope-collapsed tool called without an explicit scope
// resolves to the same operation the tool itself would have run.
func resolveBinding(
	bindings map[audit.MCPBindingKey]audit.MCPBinding, toolName string, args map[string]any,
) (audit.MCPBinding, bool) {
	if b, ok := bindings[audit.MCPBindingKey{ToolName: toolName}]; ok {
		return b, true
	}
	scope := tools.ScopeNamespace
	if argFromParsed(args, "scope") == tools.ScopeCluster {
		scope = tools.ScopeCluster
	}
	b, ok := bindings[audit.MCPBindingKey{ToolName: toolName, Scope: scope}]
	return b, ok
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

// parseCallArguments unmarshals a tools/call request's raw JSON arguments
// once, so a call needing more than one argument out of it — resolveBinding's
// "scope" and newAuditMiddleware's resource-name argument both come from the
// same request — doesn't re-parse the same bytes per argument. Returns nil if
// req is nil, carries no arguments, or the arguments aren't valid JSON.
func parseCallArguments(req mcp.Request) map[string]any {
	if req == nil {
		return nil
	}
	params, ok := req.GetParams().(*mcp.CallToolParamsRaw)
	if !ok || params == nil || len(params.Arguments) == 0 {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(params.Arguments, &raw); err != nil {
		return nil
	}
	return raw
}

// argFromParsed reads the named field out of args, as returned by
// parseCallArguments. Returns "" if the argument is absent or not a string.
func argFromParsed(args map[string]any, argName string) string {
	if v, ok := args[argName].(string); ok {
		return v
	}
	return ""
}

// extractResourceArg reads the named field out of a tools/call request's raw
// JSON arguments. Returns "" if the argument is absent or not a string.
func extractResourceArg(req mcp.Request, argName string) string {
	if argName == "" {
		return ""
	}
	return argFromParsed(parseCallArguments(req), argName)
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
