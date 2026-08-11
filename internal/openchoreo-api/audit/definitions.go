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
		// Project operations
		{
			ID: "CreateProject", Action: "create_project", ResourceType: "projects", Category: audit.CategoryManagement,
			MCPToolName: "create_project", MCPResourceArg: "name",
		},
		{
			ID: "UpdateProject", Action: "update_project", ResourceType: "projects", Category: audit.CategoryManagement,
			MCPToolName: "update_project", MCPResourceArg: "project_name",
		},
		{
			ID: "DeleteProject", Action: "delete_project", ResourceType: "projects", Category: audit.CategoryManagement,
			MCPToolName: "delete_project", MCPResourceArg: "project_name",
		},

		// DataPlane operations — no MCP tool reaches DataPlaneService's
		// create/update/delete methods.
		{ID: "CreateDataPlane", Action: "create_dataplane", ResourceType: "dataplanes", Category: audit.CategoryManagement},
		{ID: "UpdateDataPlane", Action: "update_dataplane", ResourceType: "dataplanes", Category: audit.CategoryManagement},
		{ID: "DeleteDataPlane", Action: "delete_dataplane", ResourceType: "dataplanes", Category: audit.CategoryManagement},

		// Environment operations
		{
			ID: "CreateEnvironment", Action: "create_environment", ResourceType: "environments",
			Category: audit.CategoryManagement, MCPToolName: "create_environment", MCPResourceArg: "name",
		},
		{
			ID: "UpdateEnvironment", Action: "update_environment", ResourceType: "environments",
			Category: audit.CategoryManagement, MCPToolName: "update_environment", MCPResourceArg: "name",
		},
		{
			ID: "DeleteEnvironment", Action: "delete_environment", ResourceType: "environments",
			Category: audit.CategoryManagement, MCPToolName: "delete_environment", MCPResourceArg: "name",
		},

		// Secret operations — no MCP binding; see the doc comment above.
		{ID: "CreateSecret", Action: "create_secret", ResourceType: "secrets", Category: audit.CategoryManagement},
		{ID: "UpdateSecret", Action: "update_secret", ResourceType: "secrets", Category: audit.CategoryManagement},
		{ID: "DeleteSecret", Action: "delete_secret", ResourceType: "secrets", Category: audit.CategoryManagement},
	}
}

// GetOperations returns every audited Operation the REST resolver consumes —
// operationDefs run through the generic audit.Operations derivation.
func GetOperations() []audit.Operation {
	return audit.Operations(operationDefs())
}

// MCPBindings returns the MCP tool-to-operation binding table, keyed by tool
// name — operationDefs run through the generic audit.MCPBindings derivation.
func MCPBindings() map[string]audit.MCPBinding {
	return audit.MCPBindings(operationDefs())
}
