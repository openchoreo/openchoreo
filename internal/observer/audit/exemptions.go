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

	// QueryAuditLogs is deliberately absent from this table: it is defined in
	// tools/auditgen's observer overrides under CategoryAccess, so querying the
	// trail appends to it. Filing it here with the reads below would have
	// satisfied the gate while dropping the one read the trail is meant to
	// record.
	//
	// Its sibling is exempt, and the difference is volume rather than
	// sensitivity. A filter picker populates on every interaction that changes
	// a query, so auditing it would bury the record reads it leads to — and
	// those are audited, which is where the disclosure that matters is already
	// captured. The values it returns are the trail's own vocabulary, not its
	// contents; the same permission still gates it.
	"QueryAuditLogFilterValues": "Populates a filter picker, so it fires on every " +
		"interaction that changes a query. Auditing it would bury QueryAuditLogs, " +
		"which is audited and is where the disclosure is recorded. Gated on " +
		"auditlogs:view all the same.",

	// Public spec — reads expressed as POST, to carry a query body.
	"QueryAlerts":          reasonReadAsPOST,
	"QueryDoraDeployments": reasonReadAsPOST,
	"QueryDoraMetrics":     reasonReadAsPOST,
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

// reasonTelemetryRead is why observer's signal tools are unaudited. They return
// workload telemetry rather than any account of who did what, so reading one
// discloses nothing the trail exists to record.
const reasonTelemetryRead = "Reads workload telemetry, not the record of who did what. " +
	"Unaudited on REST for the same reason."

// MCPToolExemptions maps an MCP tool name to the reason it is deliberately
// unaudited rather than bound to an operation in mcpEnrichment.
//
// Exhaustive over the tools observer registers minus the bound ones, following
// RESTExemptions rather than openchoreo-api's MCPToolExemptions. That one lists
// only tools whose declared authz action is state-modifying, leaning on a
// ToolPermission registry observer does not have; and the verb would be the
// wrong test here anyway, since query_audit_logs is a read that is audited.
//
// TestMCPToolCoverage fails for a registered tool that is neither bound nor
// listed here, so a new tool cannot go unaudited by nobody noticing.
var MCPToolExemptions = map[string]string{
	"query_component_logs":   reasonTelemetryRead,
	"query_workflow_logs":    reasonTelemetryRead,
	"query_platform_logs":    reasonTelemetryRead,
	"query_component_events": reasonTelemetryRead,
	"query_workflow_events":  reasonTelemetryRead,
	"query_resource_metrics": reasonTelemetryRead,
	"query_http_metrics":     reasonTelemetryRead,
	"query_traces":           reasonTelemetryRead,
	"query_trace_spans":      reasonTelemetryRead,
	"get_span_details":       reasonTelemetryRead,
	"query_alerts":           reasonTelemetryRead,
	"query_incidents":        reasonTelemetryRead,
	"query_costs":            reasonTelemetryRead,
	"query_recommendations":  reasonTelemetryRead,
	"query_dora_metrics":     reasonTelemetryRead,
	"query_dora_deployments": reasonTelemetryRead,
}
