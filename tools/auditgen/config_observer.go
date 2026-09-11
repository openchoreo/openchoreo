// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	observeraudit "github.com/openchoreo/openchoreo/internal/observer/audit"
	"github.com/openchoreo/openchoreo/tools/internal/auditgen"
)

// Observer serves its operations from two generated specs, so there are two
// configs. A shared one would fail: BuildDefinitions runs
// checkNoOrphanCategories against a single spec, and "incidents" is
// public-only.

// observerExcludedOperationIDs comes from internal/observer/audit rather than
// being restated here; the copy this replaced had already drifted, still
// excluding QuerySpanDetailsForTrace after it left the spec.
//
// Shared by both passes: an id from the other spec is inert, since exclusions
// are only consulted for operations the spec being walked declares.
var observerExcludedOperationIDs = excludedOperationIDs(observeraudit.RESTExemptions)

// observerPublicResourceCategories maps each resource kind a non-excluded
// public operation can target to its Category. UpdateIncident is the only
// operation deriveDefinition ever sees here, so "incidents" is the only kind
// this table needs — the two audit-log reads are overridden below and so never
// reach the derivation that consults it.
var observerPublicResourceCategories = map[string]string{
	"incidents": "CategoryManagement",
}

// observerAuditReadOverrides define the audit trail's own read, so querying the
// trail appends to it.
//
// It needs an override rather than derivation for two independent reasons.
// deriveDefinition has no verb for a read — it would map POST to "create_" and
// then reject the mismatch against the operationId's leading word. And the
// resource-kind segment it would parse from this path is "query", which names
// no resource.
//
// The read verb is "read_" and the category is CategoryAccess, both introduced
// for this: every other action is a mutation verb, and the other two categories
// mean state change and authorization change. A read of the trail is neither,
// and filing it under management would make Category useless as a filter for
// telling disclosure apart from change.
//
// QueryAuditLogFilterValues is deliberately not here — see its entry in
// internal/observer/audit's RESTExemptions.
var observerAuditReadOverrides = []auditgen.OperationDef{
	{
		ID: "QueryAuditLogs", Action: "read_audit_log", ResourceType: "auditlog",
		Category: "CategoryAccess",
	},
}

// observerInternalResourceCategories is empty because every operation on the
// internal spec is excluded, so no kind is recorded as used. When an exemption
// lifts, generation fails with "no category for kind" — the right prompt to
// decide it deliberately. Note the derived kind is "rule", not "alertrule":
// pathTail takes the last non-parameter segment of
// /alerts/sources/{sourceType}/rules/{ruleName}.
var observerInternalResourceCategories = map[string]string{}

// observerPublicConfig returns the Config for openapi/observer-api.yaml.
func observerPublicConfig() auditgen.Config {
	overrides := make(map[string]auditgen.OperationDef, len(observerAuditReadOverrides))
	for _, def := range observerAuditReadOverrides {
		overrides[def.ID] = def
	}
	return auditgen.Config{
		ResourceCategories:   observerPublicResourceCategories,
		ExcludedOperationIDs: observerExcludedOperationIDs,
		Overrides:            overrides,
	}
}

// observerInternalConfig returns the Config for
// openapi/observer-internal-api.yaml.
func observerInternalConfig() auditgen.Config {
	return auditgen.Config{
		ResourceCategories:   observerInternalResourceCategories,
		ExcludedOperationIDs: observerExcludedOperationIDs,
	}
}
