// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// mcpEnrichmentEntry sets an OperationDef's MCP fields — declared separately
// from the generated REST-only defs because the MCP tool/scope/resource-arg
// mapping isn't derivable from the OpenAPI spec: argument names are ad hoc,
// and only some identifier-carrying tools align mechanically with their REST
// path parameter.
//
// ResourceArg is "" for a handful of entries below (e.g. create_workload,
// create_release_binding, create_workflow_run) where no JSON argument in the
// tool's call carries the *created* resource's own identity — the arg named
// "name" in those calls identifies a parent (the workflow being run, the
// component the workload belongs to), not the thing this operation creates.
// Binding the tool with the wrong argument would seed resource.name with a
// misleading value; leaving it empty degrades safely to no resource.name,
// same as an unaudited body-carried REST create before its handler runs.
type mcpEnrichmentEntry struct {
	ToolName    string
	Scope       string
	ResourceArg string
}

// mcpEnrichment keys an mcpEnrichmentEntry by the REST operationId it binds
// onto. A scope-collapsed tool (one MCP tool, two REST operations) appears as
// two entries here — one per operationId, same ToolName, different Scope
// ("namespace" / "cluster", matching pkg/mcp/tools.ScopeNamespace/ScopeCluster
// — mcpaudit's adapter resolves the caller's scope argument to the same
// values before looking up a binding, see resolveBinding in
// pkg/mcp/mcpaudit/audit_middleware.go).
var mcpEnrichment = map[string]mcpEnrichmentEntry{
	"CreateProject":     {ToolName: "create_project", ResourceArg: "name"},
	"UpdateProject":     {ToolName: "update_project", ResourceArg: "project_name"},
	"DeleteProject":     {ToolName: "delete_project", ResourceArg: "project_name"},
	"CreateEnvironment": {ToolName: "create_environment", ResourceArg: "name"},
	"UpdateEnvironment": {ToolName: "update_environment", ResourceArg: "name"},
	"DeleteEnvironment": {ToolName: "delete_environment", ResourceArg: "name"},

	// Namespace — only create has an MCP tool; update/delete are REST-only.
	"CreateNamespace": {ToolName: "create_namespace", ResourceArg: "name"},

	"CreateComponent": {ToolName: "create_component", ResourceArg: "name"},
	"UpdateComponent": {ToolName: "patch_component", ResourceArg: "component_name"},
	"DeleteComponent": {ToolName: "delete_component", ResourceArg: "component_name"},
	"CreateWorkload":  {ToolName: "create_workload"}, // "name" in the call is the parent component
	"UpdateWorkload":  {ToolName: "update_workload", ResourceArg: "workload_name"},
	"DeleteWorkload":  {ToolName: "delete_workload", ResourceArg: "workload_name"},

	// ReleaseBinding — create carries no binding-name argument (the binding's
	// identity comes back from the service call, same as a body-carried REST
	// create with no RESTResourceParam).
	"CreateReleaseBinding": {ToolName: "create_release_binding"},
	"UpdateReleaseBinding": {ToolName: "update_release_binding", ResourceArg: "binding_name"},
	"DeleteReleaseBinding": {ToolName: "delete_release_binding", ResourceArg: "binding_name"},

	// ComponentRelease. GenerateRelease is bound here, not exempted: its MCP
	// tool (create_component_release) and REST's GenerateRelease handler both
	// call ComponentService.GenerateRelease, which persists a new
	// ComponentRelease via k8sClient.Create — see generateReleaseOverride's
	// doc comment in tools/auditgen/generator.go.
	"DeleteComponentRelease": {ToolName: "delete_component_release", ResourceArg: "release_name"},
	"GenerateRelease":        {ToolName: "create_component_release", ResourceArg: "release_name"},

	// WorkflowRun. create_workflow_run's "name" argument is the Workflow CR
	// being executed, not the run being created, so ResourceArg is empty.
	// trigger_workflow_run reaches the same operation — see mcpAliases.
	"CreateWorkflowRun": {ToolName: "create_workflow_run"},

	"CreateDeploymentPipeline": {ToolName: "create_deployment_pipeline", ResourceArg: "name"},
	"UpdateDeploymentPipeline": {ToolName: "update_deployment_pipeline", ResourceArg: "name"},
	"DeleteDeploymentPipeline": {ToolName: "delete_deployment_pipeline", ResourceArg: "name"},

	// ResourceRelease / ProjectRelease — update has no MCP tool on either.
	"CreateResourceRelease": {ToolName: "create_resource_release", ResourceArg: "name"},
	"DeleteResourceRelease": {ToolName: "delete_resource_release", ResourceArg: "name"},
	"CreateProjectRelease":  {ToolName: "create_project_release", ResourceArg: "name"},
	"DeleteProjectRelease":  {ToolName: "delete_project_release", ResourceArg: "name"},

	"CreateResource": {ToolName: "create_resource", ResourceArg: "name"},
	"UpdateResource": {ToolName: "update_resource", ResourceArg: "name"},
	"DeleteResource": {ToolName: "delete_resource", ResourceArg: "name"},

	"CreateResourceReleaseBinding": {ToolName: "create_resource_release_binding", ResourceArg: "name"},
	"UpdateResourceReleaseBinding": {ToolName: "update_resource_release_binding", ResourceArg: "name"},
	"DeleteResourceReleaseBinding": {ToolName: "delete_resource_release_binding", ResourceArg: "name"},
	"CreateProjectReleaseBinding":  {ToolName: "create_project_release_binding", ResourceArg: "name"},
	"UpdateProjectReleaseBinding":  {ToolName: "update_project_release_binding", ResourceArg: "name"},
	"DeleteProjectReleaseBinding":  {ToolName: "delete_project_release_binding", ResourceArg: "name"},

	"CreateSecretReference": {ToolName: "create_secret_reference", ResourceArg: "name"},
	"UpdateSecretReference": {ToolName: "update_secret_reference", ResourceArg: "secret_reference_name"},
	"DeleteSecretReference": {ToolName: "delete_secret_reference", ResourceArg: "secret_reference_name"},

	// --- Scope-collapsed families: one MCP tool, two REST operations each. ---
	// All seven use "name" as the resource-arg — registerScopedWriteTool and
	// registerScopedSingleResourceTool hardcode it (pkg/mcp/tools/scoped_helpers.go).

	"CreateComponentType":        {ToolName: "create_component_type", Scope: "namespace", ResourceArg: "name"},
	"CreateClusterComponentType": {ToolName: "create_component_type", Scope: "cluster", ResourceArg: "name"},
	"UpdateComponentType":        {ToolName: "update_component_type", Scope: "namespace", ResourceArg: "name"},
	"UpdateClusterComponentType": {ToolName: "update_component_type", Scope: "cluster", ResourceArg: "name"},
	"DeleteComponentType":        {ToolName: "delete_component_type", Scope: "namespace", ResourceArg: "name"},
	"DeleteClusterComponentType": {ToolName: "delete_component_type", Scope: "cluster", ResourceArg: "name"},

	"CreateTrait":        {ToolName: "create_trait", Scope: "namespace", ResourceArg: "name"},
	"CreateClusterTrait": {ToolName: "create_trait", Scope: "cluster", ResourceArg: "name"},
	"UpdateTrait":        {ToolName: "update_trait", Scope: "namespace", ResourceArg: "name"},
	"UpdateClusterTrait": {ToolName: "update_trait", Scope: "cluster", ResourceArg: "name"},
	"DeleteTrait":        {ToolName: "delete_trait", Scope: "namespace", ResourceArg: "name"},
	"DeleteClusterTrait": {ToolName: "delete_trait", Scope: "cluster", ResourceArg: "name"},

	"CreateWorkflow":        {ToolName: "create_workflow", Scope: "namespace", ResourceArg: "name"},
	"CreateClusterWorkflow": {ToolName: "create_workflow", Scope: "cluster", ResourceArg: "name"},
	"UpdateWorkflow":        {ToolName: "update_workflow", Scope: "namespace", ResourceArg: "name"},
	"UpdateClusterWorkflow": {ToolName: "update_workflow", Scope: "cluster", ResourceArg: "name"},
	"DeleteWorkflow":        {ToolName: "delete_workflow", Scope: "namespace", ResourceArg: "name"},
	"DeleteClusterWorkflow": {ToolName: "delete_workflow", Scope: "cluster", ResourceArg: "name"},

	"CreateResourceType":        {ToolName: "create_resource_type", Scope: "namespace", ResourceArg: "name"},
	"CreateClusterResourceType": {ToolName: "create_resource_type", Scope: "cluster", ResourceArg: "name"},
	"UpdateResourceType":        {ToolName: "update_resource_type", Scope: "namespace", ResourceArg: "name"},
	"UpdateClusterResourceType": {ToolName: "update_resource_type", Scope: "cluster", ResourceArg: "name"},
	"DeleteResourceType":        {ToolName: "delete_resource_type", Scope: "namespace", ResourceArg: "name"},
	"DeleteClusterResourceType": {ToolName: "delete_resource_type", Scope: "cluster", ResourceArg: "name"},

	"CreateProjectType":        {ToolName: "create_project_type", Scope: "namespace", ResourceArg: "name"},
	"CreateClusterProjectType": {ToolName: "create_project_type", Scope: "cluster", ResourceArg: "name"},
	"UpdateProjectType":        {ToolName: "update_project_type", Scope: "namespace", ResourceArg: "name"},
	"UpdateClusterProjectType": {ToolName: "update_project_type", Scope: "cluster", ResourceArg: "name"},
	"DeleteProjectType":        {ToolName: "delete_project_type", Scope: "namespace", ResourceArg: "name"},
	"DeleteClusterProjectType": {ToolName: "delete_project_type", Scope: "cluster", ResourceArg: "name"},

	// AuthzRole/AuthzRoleBinding — the naming break: the MCP tool family is
	// "authz_role[_binding]" but the REST operationIds are Namespace/Cluster
	// "Role"/"RoleBinding".
	"CreateNamespaceRole": {ToolName: "create_authz_role", Scope: "namespace", ResourceArg: "name"},
	"CreateClusterRole":   {ToolName: "create_authz_role", Scope: "cluster", ResourceArg: "name"},
	"UpdateNamespaceRole": {ToolName: "update_authz_role", Scope: "namespace", ResourceArg: "name"},
	"UpdateClusterRole":   {ToolName: "update_authz_role", Scope: "cluster", ResourceArg: "name"},
	"DeleteNamespaceRole": {ToolName: "delete_authz_role", Scope: "namespace", ResourceArg: "name"},
	"DeleteClusterRole":   {ToolName: "delete_authz_role", Scope: "cluster", ResourceArg: "name"},

	"CreateNamespaceRoleBinding": {ToolName: "create_authz_role_binding", Scope: "namespace", ResourceArg: "name"},
	"CreateClusterRoleBinding":   {ToolName: "create_authz_role_binding", Scope: "cluster", ResourceArg: "name"},
	"UpdateNamespaceRoleBinding": {ToolName: "update_authz_role_binding", Scope: "namespace", ResourceArg: "name"},
	"UpdateClusterRoleBinding":   {ToolName: "update_authz_role_binding", Scope: "cluster", ResourceArg: "name"},
	"DeleteNamespaceRoleBinding": {ToolName: "delete_authz_role_binding", Scope: "namespace", ResourceArg: "name"},
	"DeleteClusterRoleBinding":   {ToolName: "delete_authz_role_binding", Scope: "cluster", ResourceArg: "name"},
}

// mcpAliases covers tool names that reach an operation already bound above
// through a different, canonical tool name (the "N tools -> 1 operation"
// fan-in case) — see audit.MCPAlias's doc comment.
//
// Two shapes:
//   - trigger_workflow_run: a genuine second entry point with a different
//     argument shape, reaching the same CreateWorkflowRun operation as
//     create_workflow_run.
//   - The 9 deprecated cluster-prefixed aliases (register.go's
//     deprecatedToolNames): each routes to the cluster branch of a
//     scope-collapsed tool's canonical operation. Scope is deliberately ""
//     (not "cluster"): these tools take no scope argument at all, so
//     mcpaudit's resolveBinding must resolve them on its first, unscoped
//     lookup — giving them Scope: "cluster" here would make that lookup
//     miss and fall through to resolving scope from a call that has no
//     scope argument, defaulting to namespace and binding the wrong
//     operation.
var mcpAliases = []audit.MCPAlias{
	{OperationID: "CreateWorkflowRun", ToolName: "trigger_workflow_run"},

	{OperationID: "CreateClusterComponentType", ToolName: "create_cluster_component_type", ResourceArg: "name"},
	{OperationID: "UpdateClusterComponentType", ToolName: "update_cluster_component_type", ResourceArg: "name"},
	{OperationID: "DeleteClusterComponentType", ToolName: "delete_cluster_component_type", ResourceArg: "name"},
	{OperationID: "CreateClusterTrait", ToolName: "create_cluster_trait", ResourceArg: "name"},
	{OperationID: "UpdateClusterTrait", ToolName: "update_cluster_trait", ResourceArg: "name"},
	{OperationID: "DeleteClusterTrait", ToolName: "delete_cluster_trait", ResourceArg: "name"},
	{OperationID: "CreateClusterWorkflow", ToolName: "create_cluster_workflow", ResourceArg: "name"},
	{OperationID: "UpdateClusterWorkflow", ToolName: "update_cluster_workflow", ResourceArg: "name"},
	{OperationID: "DeleteClusterWorkflow", ToolName: "delete_cluster_workflow", ResourceArg: "name"},
}

// validateEnrichmentKeys reports an error if any key in enrichment doesn't
// match an ID in defs. mcpAliases' OperationID gets this same check for free
// from audit.MergeMCPAliases (see TestMergeMCPAliases_UnknownOperationErrors),
// but mcpEnrichment has no equivalent construction-time check elsewhere: a
// typo'd key here is simply never matched by operationDefs' lookup, so the
// intended operation silently keeps its zero-value MCP fields — no error, no
// binding, no signal beyond a tool going unexpectedly unbound.
func validateEnrichmentKeys(defs []audit.OperationDef, enrichment map[string]mcpEnrichmentEntry) error {
	valid := make(map[string]bool, len(defs))
	for _, d := range defs {
		valid[d.ID] = true
	}
	for id := range enrichment {
		if !valid[id] {
			return fmt.Errorf(
				"audit: mcpEnrichment key %q does not match any operation ID in "+
					"generatedOperationDefs or nonSpecOperationDefs — check for a typo", id)
		}
	}
	return nil
}

// init fails process startup immediately, naming the bad key, rather than
// letting a typo'd mcpEnrichment key silently ship as an unbound tool with no
// error anywhere — see validateEnrichmentKeys' doc comment.
func init() {
	all := append(generatedOperationDefs(), nonSpecOperationDefs...)
	if err := validateEnrichmentKeys(all, mcpEnrichment); err != nil {
		panic(err)
	}
}

// operationDefs is the single source of truth for every audited operation on
// both surfaces: generatedOperationDefs' REST-derived fields, enriched with
// the hand-declared MCP bindings above.
func operationDefs() []audit.OperationDef {
	defs := generatedOperationDefs()
	for i := range defs {
		if e, ok := mcpEnrichment[defs[i].ID]; ok {
			defs[i].MCPToolName = e.ToolName
			defs[i].MCPScope = e.Scope
			defs[i].MCPResourceArg = e.ResourceArg
		}
	}
	return append(defs, nonSpecOperationDefs...)
}

// GetOperations returns every audited Operation the REST resolver consumes —
// operationDefs run through the generic audit.Operations derivation.
func GetOperations() []audit.Operation {
	return audit.Operations(operationDefs())
}

// MCPBindings returns the MCP tool-to-operation binding table, keyed by
// (tool name, scope): operationDefs' embedded MCP fields via
// audit.MCPBindings, plus mcpAliases' fan-in entries via
// audit.MergeMCPAliases. The error return is defensive: both tables are
// static compile-time data, so a collision here is a bug caught by this
// package's tests long before it runs in production, not a runtime
// condition.
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
