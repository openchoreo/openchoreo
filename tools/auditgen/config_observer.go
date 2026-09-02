// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/openchoreo/openchoreo/tools/internal/auditgen"

// Observer serves its operations from two generated specs, so there are two
// configs. A shared one would fail: BuildDefinitions runs
// checkNoOrphanCategories against a single spec, and "incidents" is
// public-only. Splitting also keeps each exclusion list to operations its own
// spec can actually produce — an invariant nothing else checks.

// observerPublicExcludedOperationIDs are state-modifying operations in
// openapi/observer-api.yaml deliberately not audited — see
// internal/observer/audit/exemptions.go for each reason.
//
// All nine are reads expressed as POST. QuerySpanDetailsForTrace is POST to
// carry a request body; every other path ends in /query.
var observerPublicExcludedOperationIDs = map[string]bool{
	"QueryAlerts":              true,
	"QueryEvents":              true,
	"QueryIncidents":           true,
	"QueryLogs":                true,
	"QueryMetrics":             true,
	"QueryRuntimeTopology":     true,
	"QuerySpanDetailsForTrace": true,
	"QuerySpansForTrace":       true,
	"QueryTraces":              true,
}

// observerInternalExcludedOperationIDs are the alert-rule writes and webhook
// in openapi/observer-internal-api.yaml. All four are excluded: they run on
// the unauthenticated internal port, so there is no actor to record.
//
// That is every non-GET operation the internal spec declares, so this pass
// produces no definitions at all today. Expected, not a bug — the middleware
// is still wired there (see handlers.InternalMiddlewares), so lifting an
// exemption needs no wiring change.
var observerInternalExcludedOperationIDs = map[string]bool{
	"CreateAlertRule":    true,
	"UpdateAlertRule":    true,
	"DeleteAlertRule":    true,
	"HandleAlertWebhook": true,
}

// observerPublicResourceCategories maps each resource kind a non-excluded
// public operation can target to its Category. UpdateIncident is the only such
// operation, so "incidents" is the only kind this pass ever sees.
var observerPublicResourceCategories = map[string]string{
	"incidents": "CategoryManagement",
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
	return auditgen.Config{
		ResourceCategories:   observerPublicResourceCategories,
		ExcludedOperationIDs: observerPublicExcludedOperationIDs,
	}
}

// observerInternalConfig returns the Config for
// openapi/observer-internal-api.yaml.
func observerInternalConfig() auditgen.Config {
	return auditgen.Config{
		ResourceCategories:   observerInternalResourceCategories,
		ExcludedOperationIDs: observerInternalExcludedOperationIDs,
	}
}
