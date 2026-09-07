// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

// RESTExemptions maps a state-modifying observer-api REST operationId to the
// reason it is deliberately unaudited rather than given a definition. Every
// exemption here must carry a real reason — the coverage gate
// (TestAuditCoverage) enforces that every state-modifying operation is
// either defined or exempted, and that every exemption here still names a
// real, live operation.
var RESTExemptions = map[string]string{
	"CreateAlertRule": "Runs on the unauthenticated internal port 8081 — no JWT middleware, " +
		"no real actor to record.",
	"UpdateAlertRule": "Runs on the unauthenticated internal port 8081 — no JWT middleware, " +
		"no real actor to record.",
	"DeleteAlertRule": "Runs on the unauthenticated internal port 8081 — no JWT middleware, " +
		"no real actor to record.",
	"HandleAlertWebhook": "Runs on the unauthenticated internal port 8081 — no JWT middleware, " +
		"no real actor to record.",

	"QueryAlerts":          "A read expressed as POST, not a state-modifying action.",
	"QueryEvents":          "A read expressed as POST, not a state-modifying action.",
	"QueryIncidents":       "A read expressed as POST, not a state-modifying action.",
	"QueryLogs":            "A read expressed as POST, not a state-modifying action.",
	"QueryMetrics":         "A read expressed as POST, not a state-modifying action.",
	"QueryRuntimeTopology": "A read expressed as POST, not a state-modifying action.",
	"QuerySpansForTrace":   "A read expressed as POST, not a state-modifying action.",
	"QueryTraces":          "A read expressed as POST, not a state-modifying action.",
}

// MCPToolNames pins the tool names observer's MCP server registers
// (internal/observer/mcp's registerTools).
//
// Not a permission check: observer has no ToolPermission registry (unlike
// openchoreo-api's pkg/mcp/tools), so this cannot classify a tool as
// state-modifying the way TestAuditCoverage does for openchoreo-api. All the
// names here are read-only queries, verified by reading server.go.
//
// TestMCPToolRegistry_NoMutatingTools diffs this against the tools the server
// really registers, read back over the protocol — so adding a tool without
// listing it here fails, forcing a human to classify it.
var MCPToolNames = map[string]bool{
	"query_component_logs":   true,
	"query_workflow_logs":    true,
	"query_component_events": true,
	"query_workflow_events":  true,
	"query_resource_metrics": true,
	"query_http_metrics":     true,
	"query_traces":           true,
	"query_trace_spans":      true,
	"get_span_details":       true,
	"query_alerts":           true,
	"query_incidents":        true,
	"query_costs":            true,
	"query_recommendations":  true,
}
