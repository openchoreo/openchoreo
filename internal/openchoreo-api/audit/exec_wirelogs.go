// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// nonSpecOperationDefs are hand-declared, not generated: both routes are
// registered on a top-level mux outside the OpenAPI-generated handler (see
// cmd/openchoreo-api/main.go) and have no operationId in openapi.yaml to
// cross-reference, so NotInOpenAPISpec tells BuildPatternMap to skip them
// rather than fail construction. They still flow through operationDefs()
// like every other operation — selector validation (config/audit.go),
// TestAuditCoverage, and the coverage matrix all see them for free; only
// pattern resolution is special-cased. The caller that actually serves
// these routes owns its own pattern map keyed by the *registered* route
// pattern — see handlers.NewExecWirelogsAuditMiddleware — since that's not
// derivable from a spec that doesn't contain them.
//
// Both are audited despite being GET requests: exec is a live shell into a
// pod, wirelogs streams live traffic data, and neither is an ordinary
// resource read the ordinary "reads are out of scope" rule is about.
var nonSpecOperationDefs = []audit.OperationDef{
	{
		ID: "Exec", Action: "exec_component", ResourceType: "component",
		Category: audit.CategoryManagement, NotInOpenAPISpec: true,
	},
	{
		ID: "Wirelogs", Action: "view_wirelogs", ResourceType: "environment",
		Category: audit.CategoryManagement, RESTResourceParam: "environment", NotInOpenAPISpec: true,
	},
}
