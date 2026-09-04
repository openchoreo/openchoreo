// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

// RESTExemptions maps a state-modifying REST operationId to the reason it is
// deliberately unaudited rather than given a definition. Every exemption here
// must carry a real reason — the coverage gate (mcphandlers' TestAuditCoverage)
// enforces that every state-modifying operation is either defined or exempted,
// and that every exemption here still names a real, live operation.
//
// GenerateRelease is NOT here: it persists a new ComponentRelease via
// k8sClient.Create (see ComponentService.GenerateRelease), so it has a real
// definition (generatedOperationDefs' generateReleaseOverride) instead.
var RESTExemptions = map[string]string{
	"Evaluates": "Evaluates an authz decision; a read expressed as POST, not a state-modifying action.",
	"HandleAutoBuild": "Invoked via an HMAC-authenticated webhook, not a user action — there is no " +
		"particular actor to capture for the event.",
}

// MCPToolExemptions maps an MCP tool name to the reason it is deliberately
// unaudited despite declaring a create/update/delete authz action. These are
// get_*_creation_schema tools that gate on a mutating action purely for
// permission purposes (only a subject who could create the resource may see
// its creation schema) but make no service call and mutate nothing — see
// pkg/mcp/tools/scoped_component_types.go's RegisterGetComponentTypeCreationSchema
// and its siblings. Every other state-modifying MCP tool must be bound via
// MCPBindings, not exempted.
var MCPToolExemptions = map[string]string{
	"get_component_type_creation_schema": "Declares componenttype:create for permission-gating only; " +
		"returns a static JSON schema and calls no service method.",
	"get_trait_creation_schema": "Declares trait:create for permission-gating only; returns a static " +
		"JSON schema and calls no service method.",
	"get_workflow_creation_schema": "Declares workflow:create for permission-gating only; returns a " +
		"static JSON schema and calls no service method.",
	"get_resource_type_creation_schema": "Declares resourcetype:create for permission-gating only; " +
		"returns a static JSON schema and calls no service method.",
	"get_project_type_creation_schema": "Declares projecttype:create for permission-gating only; " +
		"returns a static JSON schema and calls no service method.",

	// Deprecated cluster-prefixed aliases of three of the five above — same
	// exemption reason.
	"get_cluster_component_type_creation_schema": "Deprecated alias of get_component_type_creation_schema; " +
		"declares clustercomponenttype:create for permission-gating only, calls no service method.",
	"get_cluster_trait_creation_schema": "Deprecated alias of get_trait_creation_schema; declares " +
		"clustertrait:create for permission-gating only, calls no service method.",
	"get_cluster_workflow_creation_schema": "Deprecated alias of get_workflow_creation_schema; declares " +
		"clusterworkflow:create for permission-gating only, calls no service method.",
}
