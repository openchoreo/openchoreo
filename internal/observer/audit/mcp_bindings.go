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

// mcpEnrichment binds observer's MCP tools onto audited operations. Every
// other tool is listed in MCPToolExemptions.
//
// ResourceArg is empty where the tool's arguments filter many records rather
// than name one resource.
var mcpEnrichment = map[string]mcpEnrichmentEntry{
	"QueryAuditLogs":         {ToolName: "query_audit_logs"},
	"QueryLogs":              {ToolName: "query_component_logs"},
	"GetPlatformLogs":        {ToolName: "query_platform_logs"},
	"QuerySpansForTrace":     {ToolName: "query_trace_spans", ResourceArg: "trace_id"},
	"GetSpanDetailsForTrace": {ToolName: "get_span_details", ResourceArg: "trace_id"},
}

// mcpAliases covers a second tool name reaching an operation bound above.
var mcpAliases = []audit.MCPAlias{
	{OperationID: "QueryLogs", ToolName: "query_workflow_logs"},
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
// (tool name, scope), with mcpAliases merged in. Observer has no
// scope-collapsed tool, so every key here carries the empty scope.
func MCPBindings() (map[audit.MCPBindingKey]audit.MCPBinding, error) {
	defs := operationDefs()
	bindings, err := audit.MCPBindings(defs)
	if err != nil {
		return nil, err
	}
	if err := audit.MergeMCPAliases(defs, bindings, mcpAliases); err != nil {
		return nil, err
	}
	return bindings, nil
}
