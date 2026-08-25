// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// operationDefs is the single source of truth for every audited operation on
// both surfaces. Only state-modifying operations (POST, PUT, PATCH, DELETE)
// are audited.
//
// ID is the OpenAPI operationId, e.g. "CreateProject" — PascalCase, matching
// the generated Go handler method names, not the source spec's lowerCamelCase.
//
// This covers 12 of 114 state-modifying routes; the rest (and a CI gate to
// keep coverage honest) are P1 work. Only 6 of these 12 have an MCP tool: no
// MCP tool reaches DataPlaneService, and the secret-shaped MCP tools call
// SecretReferenceService — a different resource — so they're not bound here.
func operationDefs() []audit.OperationDef {
	return []audit.OperationDef{
		// Project operations.
		{
			ID: "CreateProject", Action: "create_project", ResourceType: "project", Category: audit.CategoryManagement,
			MCPToolName: "create_project", MCPResourceArg: "name",
		},
		{
			ID: "UpdateProject", Action: "update_project", ResourceType: "project", Category: audit.CategoryManagement,
			MCPToolName: "update_project", MCPResourceArg: "project_name", RESTResourceParam: "projectName",
		},
		{
			ID: "DeleteProject", Action: "delete_project", ResourceType: "project", Category: audit.CategoryManagement,
			MCPToolName: "delete_project", MCPResourceArg: "project_name", RESTResourceParam: "projectName",
		},

		// DataPlane operations — no MCP tool reaches DataPlaneService's
		// create/update/delete methods.
		{ID: "CreateDataPlane", Action: "create_dataplane", ResourceType: "dataplane", Category: audit.CategoryManagement},
		{
			ID: "UpdateDataPlane", Action: "update_dataplane", ResourceType: "dataplane",
			Category: audit.CategoryManagement, RESTResourceParam: "dpName",
		},
		{
			ID: "DeleteDataPlane", Action: "delete_dataplane", ResourceType: "dataplane",
			Category: audit.CategoryManagement, RESTResourceParam: "dpName",
		},

		// Environment operations
		{
			ID: "CreateEnvironment", Action: "create_environment", ResourceType: "environment",
			Category: audit.CategoryManagement, MCPToolName: "create_environment", MCPResourceArg: "name",
		},
		{
			ID: "UpdateEnvironment", Action: "update_environment", ResourceType: "environment",
			Category: audit.CategoryManagement, MCPToolName: "update_environment", MCPResourceArg: "name",
			RESTResourceParam: "envName",
		},
		{
			ID: "DeleteEnvironment", Action: "delete_environment", ResourceType: "environment",
			Category: audit.CategoryManagement, MCPToolName: "delete_environment", MCPResourceArg: "name",
			RESTResourceParam: "envName",
		},

		// Secret operations — no MCP binding; see the doc comment above.
		{ID: "CreateSecret", Action: "create_secret", ResourceType: "secret", Category: audit.CategoryManagement},
		{
			ID: "UpdateSecret", Action: "update_secret", ResourceType: "secret",
			Category: audit.CategoryManagement, RESTResourceParam: "secretName",
		},
		{
			ID: "DeleteSecret", Action: "delete_secret", ResourceType: "secret",
			Category: audit.CategoryManagement, RESTResourceParam: "secretName",
		},
	}
}

// GetOperations returns every audited Operation the REST resolver consumes —
// operationDefs run through the generic audit.Operations derivation.
func GetOperations() []audit.Operation {
	return audit.Operations(operationDefs())
}

// MCPBindings returns the MCP tool-to-operation binding table, keyed by
// (tool name, scope) — operationDefs run through the generic
// audit.MCPBindings derivation. The error return is defensive: operationDefs
// is a static compile-time table, so a collision here is a bug caught by
// TestMCPBindings_DerivedFromDefs / TestMCPBindings_MatchRegisteredTools long
// before this runs in production, not a runtime condition.
func MCPBindings() (map[audit.MCPBindingKey]audit.MCPBinding, error) {
	return audit.MCPBindings(operationDefs())
}
