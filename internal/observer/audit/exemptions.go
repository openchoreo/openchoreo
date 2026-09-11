// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

const (
	reasonRead       = "A read; returns data without modifying state."
	reasonReadAsPOST = "A read expressed as POST, not a state-modifying action."
	reasonInternal   = "Runs on the unauthenticated internal port 8081 — no JWT middleware, " +
		"no real actor to record."
)

// RESTExemptions maps an observer REST operationId, across both specs, to the
// reason it is deliberately unaudited rather than given a definition.
//
// Exhaustive, reads included: the coverage gate (TestAuditCoverage) fails for
// an operation that is neither defined nor listed here, so a new endpoint
// cannot go unaudited by nobody noticing. Reads are listed rather than
// inferred from the method because the method is not a reliable proxy either
// way — every Query* below is a POST that reads, and a GET can be worth
// auditing, such as reading the audit trail itself.
var RESTExemptions = map[string]string{
	// Internal spec — the unauthenticated port.
	"CreateAlertRule":    reasonInternal,
	"UpdateAlertRule":    reasonInternal,
	"DeleteAlertRule":    reasonInternal,
	"HandleAlertWebhook": reasonInternal,

	// Public spec — reads expressed as POST, to carry a query body.
	"QueryAlerts":          reasonReadAsPOST,
	"QueryEvents":          reasonReadAsPOST,
	"QueryIncidents":       reasonReadAsPOST,
	"QueryLogs":            reasonReadAsPOST,
	"QueryMetrics":         reasonReadAsPOST,
	"QueryRuntimeTopology": reasonReadAsPOST,
	"QuerySpansForTrace":   reasonReadAsPOST,
	"QueryTraces":          reasonReadAsPOST,

	// Public spec — GET.
	"GetComponentCosts":                 reasonRead,
	"GetOAuthProtectedResourceMetadata": reasonRead,
	"GetPlatformLogFilterValues":        reasonRead,
	"GetPlatformLogs":                   reasonRead,
	"GetRecommendations":                reasonRead,
	"GetSpanDetailsForTrace":            reasonRead,
	"Health":                            reasonRead,

	// Internal spec — GET.
	"GetAlertRule": reasonRead,
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
