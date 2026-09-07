// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"fmt"
	"sort"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// GetOperations returns every audited Operation across both of observer's
// generated specs. Observer has no mutating MCP tools (see MCPToolNames), so
// unlike openchoreo-api there is no MCP bindings step: generatedOperationDefs()
// is the complete table. Middleware wiring uses OperationsIn instead, since
// each port needs only its own spec's share.
func GetOperations() []audit.Operation {
	return audit.Operations(generatedOperationDefs())
}

// OperationsIn returns the subset of GetOperations() whose operationId
// swagger declares. Each port builds its pattern map from its own spec, and
// audit.BuildPatternMap errors on an operationId with no matching route — so
// passing the whole table to either port would fail startup.
//
// Filtering rather than hand-splitting the table means a lifted exemption
// needs only a regeneration. TestEveryDefinedOperationBelongsToExactlyOneSpec
// guards against an operation silently matching neither spec.
func OperationsIn(swagger *openapi3.T) []audit.Operation {
	declared := make(map[string]bool)
	for _, path := range swagger.Paths.InMatchingOrder() {
		for _, op := range swagger.Paths.Find(path).Operations() {
			if op.OperationID != "" {
				declared[op.OperationID] = true
			}
		}
	}

	all := GetOperations()
	subset := make([]audit.Operation, 0, len(all))
	for _, op := range all {
		if declared[op.ID] {
			subset = append(subset, op)
		}
	}
	return subset
}

// VerifyOperationsPartition checks that the given specs together account for
// every operation in GetOperations(), each exactly once. OperationsIn
// silently drops an operation whose ID matches no spec — what a renamed
// operationId without a regenerated definitions.gen.go produces, leaving it
// unaudited with no request-time error. Calling this at startup turns that
// into a refusal to boot instead.
func VerifyOperationsPartition(swaggers ...*openapi3.T) error {
	counts := make(map[string]int)
	for _, swagger := range swaggers {
		for _, op := range OperationsIn(swagger) {
			counts[op.ID]++
		}
	}

	var missing, duplicated []string
	for _, op := range GetOperations() {
		switch counts[op.ID] {
		case 1:
		case 0:
			missing = append(missing, op.ID)
		default:
			duplicated = append(duplicated, op.ID)
		}
	}
	sort.Strings(missing)
	sort.Strings(duplicated)

	if len(missing) > 0 {
		return fmt.Errorf("audited operations %v are declared by none of the served specs, so they "+
			"would never be audited — regenerate definitions.gen.go (make audit-gen)", missing)
	}
	if len(duplicated) > 0 {
		return fmt.Errorf("audited operations %v are declared by more than one served spec", duplicated)
	}
	return nil
}
