// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

const (
	reasonRead       = "A read; returns data without modifying state."
	reasonReadAsPOST = "A read expressed as POST, not a state-modifying action."
)

// RESTExemptions maps a REST operationId to the reason it is deliberately
// unaudited rather than given a definition.
//
// Exhaustive, reads included: the coverage gate (mcphandlers'
// TestAuditCoverage) fails for an operation that is neither defined nor listed
// here, so a new endpoint cannot go unaudited by nobody noticing. Reads are
// listed rather than inferred from the method because the method is not a
// reliable proxy either way — Evaluates is a POST that reads, and GetSecret is
// a GET that might be worth auditing.
//
// GenerateRelease is NOT here: it persists a new ComponentRelease via
// k8sClient.Create (see ComponentService.GenerateRelease), so it has a real
// definition (generatedOperationDefs' generateReleaseOverride) instead.
var RESTExemptions = map[string]string{
	"HandleAutoBuild": "Invoked via an HMAC-authenticated webhook, not a user action — there is no " +
		"particular actor to capture for the event.",

	"Evaluates": reasonReadAsPOST,

	"GetClusterComponentType":                     reasonRead,
	"GetClusterComponentTypeSchema":               reasonRead,
	"GetClusterDataPlane":                         reasonRead,
	"GetClusterObservabilityPlane":                reasonRead,
	"GetClusterProjectType":                       reasonRead,
	"GetClusterProjectTypeSchema":                 reasonRead,
	"GetClusterResourceType":                      reasonRead,
	"GetClusterResourceTypeSchema":                reasonRead,
	"GetClusterRole":                              reasonRead,
	"GetClusterRoleBinding":                       reasonRead,
	"GetClusterTrait":                             reasonRead,
	"GetClusterTraitSchema":                       reasonRead,
	"GetClusterWorkflow":                          reasonRead,
	"GetClusterWorkflowPlane":                     reasonRead,
	"GetClusterWorkflowSchema":                    reasonRead,
	"GetComponent":                                reasonRead,
	"GetComponentRelease":                         reasonRead,
	"GetComponentSchema":                          reasonRead,
	"GetComponentType":                            reasonRead,
	"GetComponentTypeSchema":                      reasonRead,
	"GetDataPlane":                                reasonRead,
	"GetDeploymentPipeline":                       reasonRead,
	"GetEnvironment":                              reasonRead,
	"GetHealth":                                   reasonRead,
	"GetNamespace":                                reasonRead,
	"GetNamespaceRole":                            reasonRead,
	"GetNamespaceRoleBinding":                     reasonRead,
	"GetOAuthProtectedResourceMetadata":           reasonRead,
	"GetObservabilityAlertsNotificationChannel":   reasonRead,
	"GetObservabilityPlane":                       reasonRead,
	"GetOpenAPISpec":                              reasonRead,
	"GetProject":                                  reasonRead,
	"GetProjectRelease":                           reasonRead,
	"GetProjectReleaseBinding":                    reasonRead,
	"GetProjectType":                              reasonRead,
	"GetProjectTypeSchema":                        reasonRead,
	"GetReady":                                    reasonRead,
	"GetReleaseBinding":                           reasonRead,
	"GetReleaseBindingK8sResourceEvents":          reasonRead,
	"GetReleaseBindingK8sResourceLogs":            reasonRead,
	"GetReleaseBindingK8sResourceTree":            reasonRead,
	"GetResource":                                 reasonRead,
	"GetResourceRelease":                          reasonRead,
	"GetResourceReleaseBinding":                   reasonRead,
	"GetResourceType":                             reasonRead,
	"GetResourceTypeSchema":                       reasonRead,
	"GetSecret":                                   reasonRead,
	"GetSecretReference":                          reasonRead,
	"GetSubjectProfile":                           reasonRead,
	"GetTrait":                                    reasonRead,
	"GetTraitSchema":                              reasonRead,
	"GetVersion":                                  reasonRead,
	"GetWorkflow":                                 reasonRead,
	"GetWorkflowPlane":                            reasonRead,
	"GetWorkflowRun":                              reasonRead,
	"GetWorkflowRunEvents":                        reasonRead,
	"GetWorkflowRunLogs":                          reasonRead,
	"GetWorkflowRunStatus":                        reasonRead,
	"GetWorkflowSchema":                           reasonRead,
	"GetWorkload":                                 reasonRead,
	"ListActions":                                 reasonRead,
	"ListClusterComponentTypes":                   reasonRead,
	"ListClusterDataPlanes":                       reasonRead,
	"ListClusterObservabilityPlanes":              reasonRead,
	"ListClusterProjectTypes":                     reasonRead,
	"ListClusterResourceTypes":                    reasonRead,
	"ListClusterRoleBindings":                     reasonRead,
	"ListClusterRoles":                            reasonRead,
	"ListClusterTraits":                           reasonRead,
	"ListClusterWorkflowPlanes":                   reasonRead,
	"ListClusterWorkflows":                        reasonRead,
	"ListComponentReleases":                       reasonRead,
	"ListComponentTypes":                          reasonRead,
	"ListComponents":                              reasonRead,
	"ListDataPlanes":                              reasonRead,
	"ListDeploymentPipelines":                     reasonRead,
	"ListEnvironments":                            reasonRead,
	"ListGitSecrets":                              reasonRead,
	"ListNamespaceRoleBindings":                   reasonRead,
	"ListNamespaceRoles":                          reasonRead,
	"ListNamespaces":                              reasonRead,
	"ListObservabilityAlertsNotificationChannels": reasonRead,
	"ListObservabilityPlanes":                     reasonRead,
	"ListProjectReleaseBindings":                  reasonRead,
	"ListProjectReleases":                         reasonRead,
	"ListProjectTypes":                            reasonRead,
	"ListProjects":                                reasonRead,
	"ListReleaseBindings":                         reasonRead,
	"ListResourceReleaseBindings":                 reasonRead,
	"ListResourceReleases":                        reasonRead,
	"ListResourceTypes":                           reasonRead,
	"ListResources":                               reasonRead,
	"ListSecretReferences":                        reasonRead,
	"ListSecrets":                                 reasonRead,
	"ListSubjectTypes":                            reasonRead,
	"ListTraits":                                  reasonRead,
	"ListWorkflowPlanes":                          reasonRead,
	"ListWorkflowRuns":                            reasonRead,
	"ListWorkflows":                               reasonRead,
	"ListWorkloads":                               reasonRead,
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
