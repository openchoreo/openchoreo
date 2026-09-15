// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// mcpEnrichmentEntry sets an OperationDef's MCP fields, keyed below by the REST
// operationId it binds onto. Declared by hand rather than generated, for the
// same reason as openchoreo-api's: a tool's argument names are not derivable
// from the OpenAPI spec.
type mcpEnrichmentEntry struct {
	ToolName    string
	ResourceArg string
}

// mcpEnrichment binds observer's MCP tools onto audited operations.
//
// One entry, and it is the point of the table: every other observer tool reads
// workload telemetry and is unaudited on both surfaces (see RESTExemptions),
// while reading the audit trail is recorded wherever it happens.
//
// ResourceArg is empty because the tool targets no single resource. Its
// resource_* arguments are filters over many records, and its REST counterpart
// carries no RESTResourceParam either.
var mcpEnrichment = map[string]mcpEnrichmentEntry{
	"QueryAuditLogs": {ToolName: "query_audit_logs"},
}

// validateEnrichmentKeys reports an error if any key in enrichment doesn't
// match an ID in defs. A typo'd key is otherwise never matched by
// operationDefs' lookup, leaving the operation with zero-value MCP fields and
// a tool that reads the trail without appending to it.
func validateEnrichmentKeys(defs []audit.OperationDef, enrichment map[string]mcpEnrichmentEntry) error {
	valid := make(map[string]bool, len(defs))
	for _, d := range defs {
		valid[d.ID] = true
	}
	for id := range enrichment {
		if !valid[id] {
			return fmt.Errorf(
				"audit: mcpEnrichment key %q does not match any operation ID in "+
					"generatedOperationDefs — check for a typo", id)
		}
	}
	return nil
}

// init fails process startup rather than letting a typo'd mcpEnrichment key
// ship as an unbound tool. See validateEnrichmentKeys.
func init() {
	if err := validateEnrichmentKeys(generatedOperationDefs(), mcpEnrichment); err != nil {
		panic(err)
	}
}

// operationDefs is every audited observer operation on both surfaces: the
// generated REST-derived fields, enriched with the MCP bindings above.
func operationDefs() []audit.OperationDef {
	defs := generatedOperationDefs()
	for i := range defs {
		if e, ok := mcpEnrichment[defs[i].ID]; ok {
			defs[i].MCPToolName = e.ToolName
			defs[i].MCPResourceArg = e.ResourceArg
		}
	}
	return defs
}

// MCPBindings returns the MCP tool-to-operation binding table, keyed by
// (tool name, scope). Observer has no scope-collapsed tool, so every key here
// carries the empty scope.
func MCPBindings() (map[audit.MCPBindingKey]audit.MCPBinding, error) {
	return audit.MCPBindings(operationDefs())
}
