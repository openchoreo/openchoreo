// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	apiaudit "github.com/openchoreo/openchoreo/internal/openchoreo-api/audit"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/mcphandlers"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// registerAllToolsets enables every toolset regardless of what a given
// deployment's config turns on, since this document describes what exists in
// code, not one deployment's runtime configuration.
func registerAllToolsets() map[string]tools.ToolPermission {
	handler := mcphandlers.NewMCPHandler(&handlerservices.Services{})
	toolsets := tools.NewToolsets(handler, tools.AllToolsetTypes())
	server := mcp.NewServer(&mcp.Implementation{Name: "auditcoverage", Version: "0.0.0"}, nil)
	perms, _ := toolsets.Register(server)
	return perms
}

// renderAPIMCPSection is openchoreo-api's extraSections hook. Observer has
// none — it registers no mutating MCP tools.
func renderAPIMCPSection() (string, error) {
	bindings, err := apiaudit.MCPBindings()
	if err != nil {
		return "", fmt.Errorf("apiaudit.MCPBindings: %w", err)
	}
	return renderMCPSection(registerAllToolsets(), bindings), nil
}

// boundRow is one (tool, scope) binding — a scope-collapsed tool contributes
// two rows, one per operation it fans out to. Rendering per-binding rather
// than per-tool-name is what makes this deterministic: a map keyed on tool
// name alone can hold only one of a scope-collapsed tool's two bindings, and
// which one survives depends on Go's randomized map iteration order.
type boundRow struct {
	tool, scope, opID, action, identity string
}

// mcpResourceIdentity describes where a bound tool's audit event gets its
// resource identity, mirroring the REST table's column. The two halves come
// from different places: resource.name is seeded from the call's raw arguments
// by mcpaudit's middleware before the handler runs (so it survives a denial),
// while resource.id can only ever come from the handler recording the persisted
// object — see mcphandlers.setAuditResource. A binding with no ResourceArg has no
// argument naming the resource at all, so both fields wait on the handler.
func mcpResourceIdentity(resourceArg string) string {
	if resourceArg == "" {
		return "handler-supplied on success (the call carries no argument naming this resource); " +
			"no name or id on a denied/failed call"
	}
	return fmt.Sprintf(
		"name from the `%s` argument, available even on denial; id added by the handler on success",
		resourceArg,
	)
}

func renderMCPSection(perms map[string]tools.ToolPermission, bindings map[audit.MCPBindingKey]audit.MCPBinding) string {
	boundToolNames := make(map[string]bool, len(bindings))
	rows := make([]boundRow, 0, len(bindings))
	for key, b := range bindings {
		boundToolNames[key.ToolName] = true
		rows = append(rows, boundRow{
			tool:     key.ToolName,
			scope:    key.Scope,
			opID:     b.Operation.ID,
			action:   b.Operation.Action,
			identity: mcpResourceIdentity(b.ResourceArg),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].tool != rows[j].tool {
			return rows[i].tool < rows[j].tool
		}
		return rows[i].scope < rows[j].scope
	})

	var exempted, readOnly []string
	for name, perm := range perms {
		if boundToolNames[name] {
			continue
		}
		if perm.IsReadOnly() {
			readOnly = append(readOnly, name)
		} else {
			exempted = append(exempted, name)
		}
	}
	sort.Strings(exempted)

	out := fmt.Sprintf("## MCP — %d registered tools (%d state-modifying, %d read-only)\n\n",
		len(perms), len(boundToolNames)+len(exempted), len(readOnly))

	out += fmt.Sprintf("### Bound (%d tools, %d bindings)\n\n", len(boundToolNames), len(rows))
	out += "| Tool | Scope | Operation ID | Action | Resource identity |\n|---|---|---|---|---|\n"
	for _, r := range rows {
		scope := r.scope
		if scope == "" {
			scope = "—"
		}
		out += fmt.Sprintf("| `%s` | %s | %s | `%s` | %s |\n", r.tool, scope, r.opID, r.action, r.identity)
	}
	out += "\n"

	out += fmt.Sprintf("### Exempted (%d)\n\n", len(exempted))
	out += "| Tool | Reason |\n|---|---|\n"
	for _, name := range exempted {
		reason := apiaudit.MCPToolExemptions[name]
		if reason == "" {
			reason = "UNDOCUMENTED — declares a non-view action but has neither a binding nor an exemption reason"
		}
		out += fmt.Sprintf("| `%s` | %s |\n", name, reason)
	}
	out += "\n"

	out += fmt.Sprintf(
		"%d read-only tools (every declared action ends in `:view`) need no explicit exemption.\n\n",
		len(readOnly),
	)
	return out
}

// apiKnownNonEvents is openchoreo-api's closing narrative — hand-written notes
// about this service's specific gaps, deliberately not shared with observer's.
const apiKnownNonEvents = "## Known non-events\n\n" +
	"Cases where a real state-modifying request produces no audit event, or a diminished one, " +
	"by design or by a documented, unfixed gap:\n\n" +
	"- **Reads are out of scope.** Every GET operation on both surfaces, and MCP's `initialize` " +
	"session handshake.\n" +
	"- **A malformed path parameter never reaches audit on REST.** oapi-codegen's generated " +
	"wrapper binds path params and returns 400 before `HandlerMiddlewares` run " +
	"(`server.gen.go`), so a 400 from bad path binding is never audited.\n" +
	"- **`source_ip` is unobtainable for a direct (non-proxied) MCP client.** " +
	"`RequestExtra` carries no `RemoteAddr`; only `X-Forwarded-For`/`X-Real-IP` from " +
	"`Extra.Header` are available, and a direct client sets neither.\n" +
	"- **`X-Request-ID` and `source_ip` are both trusted from the client with no proxy check.** " +
	"`X-Request-ID` is hardened to require a parseable UUID (falling back to a generated one, " +
	"see `audit.RequestIDRejections`); `source_ip`'s `X-Forwarded-For`/`X-Real-IP` are taken at " +
	"face value — deploy behind a proxy that strips or overwrites both if `source_ip` needs to " +
	"be forensically trustworthy.\n" +
	"- **An invalid `scope` argument on a scope-collapsed MCP tool attributes the call to the " +
	"namespace-scoped operation with `result: failure`** — the tool call itself errors before " +
	"resolving a scope, so the event records the tool's default binding rather than the scope " +
	"the caller asked for. Harmless (nothing is silently permitted), just a label to be aware of " +
	"when reading a failure event's operation_id.\n" +
	"- **`pkg/mcp.NewSTDIO` registers no middleware** — neither audit nor the authz filter. Sound " +
	"today (there is no HTTP request and so no identity to attribute an event to), and it has no " +
	"production caller in this repo, but it is exported: whatever first wires MCP-over-stdio " +
	"gets an unaudited mutating surface until that's addressed.\n" +
	"- **`?filterByAuthz=false` records a service-layer authorization denial as `failure`, not " +
	"`denied`.** Opening a session with that query parameter skips the MCP authz filter " +
	"entirely, so a refusal surfaces as a `services.ErrForbidden` returned from the handler " +
	"rather than one of the filter's own sentinels. It cannot be recovered in " +
	"`mcpaudit.classifyResult`: the go-sdk's typed-tool adapter calls `CallToolResult.SetError(err)` " +
	"and returns a **nil** error to its caller, so by the time the receiving middleware runs the " +
	"original error's identity is gone and only `IsError` remains. Fixing it means translating " +
	"`services.ErrForbidden` in every MCP tool handler, or returning a structured `*jsonrpc.Error` " +
	"(which changes client-visible protocol behavior) — so a `results: [denied]` query on the " +
	"read path silently misses these.\n" +
	"- **An authentication rejection carries no `action`, `category` or `operation_id`.** On both " +
	"surfaces `NewUnauthenticatedMiddleware` emits with a nil Operation, so every " +
	"operation-derived policy selector short-circuits. Such an event is selectable only by " +
	"`origins`, `results`, `actor_types` and `actors`.\n"
