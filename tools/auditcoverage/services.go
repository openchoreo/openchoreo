// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"sort"

	observeraudit "github.com/openchoreo/openchoreo/internal/observer/audit"
	apiaudit "github.com/openchoreo/openchoreo/internal/openchoreo-api/audit"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// service is one API server's reporting inputs.
//
// The REST table is shared; the rest is per-service. Only openchoreo-api has
// an MCP section, and knownNonEvents is hand-written narrative about one
// service's gaps, which a shared renderer could only flatten.
type service struct {
	// title is the matrix's H1.
	title string
	// defaultOut is where this service's matrix is committed.
	defaultOut string
	// makeTarget is quoted in the freshness test's failure message.
	makeTarget string
	// operations and exemptions are the REST table's inputs.
	operations func() []audit.Operation
	exemptions map[string]string
	// extraSections renders between the REST table and the known non-events.
	// nil when the service has none.
	extraSections  func() (string, error)
	knownNonEvents string
}

// services is the registry the -service flag selects from.
var services = map[string]service{
	"openchoreo-api": {
		title:          "Audit Coverage Matrix",
		defaultOut:     "docs/audit/coverage-matrix.md",
		makeTarget:     "audit-coverage-matrix",
		operations:     apiaudit.GetOperations,
		exemptions:     apiaudit.RESTExemptions,
		extraSections:  renderAPIMCPSection,
		knownNonEvents: apiKnownNonEvents,
	},
	"observer": {
		title:      "Observer Audit Coverage Matrix",
		defaultOut: "docs/audit/observer-coverage-matrix.md",
		makeTarget: "audit-coverage-matrix",
		operations: observeraudit.GetOperations,
		exemptions: observeraudit.RESTExemptions,
		// No MCP section: observer has no mutating MCP tools (see
		// internal/observer/audit's MCPToolNames).
		extraSections:  nil,
		knownNonEvents: observerKnownNonEvents,
	},
}

// serviceNames returns the registry's keys in sorted order, for flag help and
// error messages.
func serviceNames() []string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
