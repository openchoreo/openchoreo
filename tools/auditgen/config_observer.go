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
		ExcludedOperationIDs: observerExcludedOperationIDs,
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
