// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// namespaceToolRegistrations returns the list of namespace toolset registration functions
func (t *Toolsets) namespaceToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterListNamespaces,
		t.RegisterCreateNamespace,
		t.RegisterListSecretReferences,
		t.RegisterGetSecretReference,
	}
}

// projectToolRegistrations returns the list of project toolset registration functions
func (t *Toolsets) projectToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterListProjects,
		t.RegisterCreateProject,
		t.RegisterUpdateProject,
		t.RegisterDeleteProject,
		t.RegisterListProjectTypes,
		t.RegisterGetProjectType,
		t.RegisterGetProjectTypeSchema,
	}
}

// componentToolRegistrations returns the list of component toolset registration functions
func (t *Toolsets) componentToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterCreateComponent,
		t.RegisterListComponents,
		t.RegisterGetComponent,
		t.RegisterPatchComponent,
		t.RegisterDeleteComponent,
		t.RegisterListWorkloads,
		t.RegisterGetWorkload,
		t.RegisterCreateWorkload,
		t.RegisterUpdateWorkload,
		t.RegisterDeleteWorkload,
		t.RegisterGetWorkloadSchema,
		t.RegisterGetComponentSchema,
		// Platform standards (read-only). These are scope-collapsed: pass scope="cluster"
		// to operate on the platform-wide cluster-scoped resource.
		t.RegisterListComponentTypes,
		t.RegisterGetComponentType,
		t.RegisterGetComponentTypeSchema,
		t.RegisterListTraits,
		t.RegisterGetTrait,
		t.RegisterGetTraitSchema,
	}
}

// deploymentToolRegistrations returns the list of deployment toolset registration functions
func (t *Toolsets) deploymentToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterListReleaseBindings,
		t.RegisterGetReleaseBinding,
		t.RegisterCreateReleaseBinding,
		t.RegisterUpdateReleaseBinding,
		t.RegisterDeleteReleaseBinding,
		t.RegisterDeleteComponentRelease,
		t.RegisterDeleteResourceRelease,
		t.RegisterDeleteProjectRelease,
		t.RegisterListResourceReleaseBindings,
		t.RegisterGetResourceReleaseBinding,
		t.RegisterCreateResourceReleaseBinding,
		t.RegisterUpdateResourceReleaseBinding,
		t.RegisterDeleteResourceReleaseBinding,
		t.RegisterListProjectReleaseBindings,
		t.RegisterGetProjectReleaseBinding,
		t.RegisterCreateProjectReleaseBinding,
		t.RegisterUpdateProjectReleaseBinding,
		t.RegisterDeleteProjectReleaseBinding,
		t.RegisterListDeploymentPipelines,
		t.RegisterGetDeploymentPipeline,
		t.RegisterListEnvironments,
	}
}

// buildToolRegistrations returns the list of build toolset registration functions
func (t *Toolsets) buildToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterTriggerWorkflowRun,
		t.RegisterCreateWorkflowRun,
		t.RegisterListWorkflowRuns,
		t.RegisterGetWorkflowRun,
		t.RegisterGetWorkflowRunStatus,
		t.RegisterGetWorkflowRunLogs,
		t.RegisterGetWorkflowRunEvents,
		// Workflow read. Scope-collapsed: pass scope="cluster" for a platform-wide ClusterWorkflow.
		t.RegisterListWorkflows,
		t.RegisterGetWorkflow,
		t.RegisterGetWorkflowSchema,
	}
}

// peToolRegistrations returns the list of pe toolset registration functions
func (t *Toolsets) peToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		// Environment management
		t.RegisterPEListEnvironments,
		t.RegisterCreateEnvironment,
		t.RegisterUpdateEnvironment,
		t.RegisterDeleteEnvironment,

		// Deployment pipeline management
		t.RegisterCreateDeploymentPipeline,
		t.RegisterUpdateDeploymentPipeline,
		t.RegisterDeleteDeploymentPipeline,

		// Component releases
		t.RegisterPEListComponentReleases,
		t.RegisterPECreateComponentRelease,
		t.RegisterPEGetComponentRelease,
		t.RegisterPEGetComponentReleaseSchema,

		// Resource releases (admin: list/get/create; delete lives on the deployment toolset).
		t.RegisterPEListResourceReleases,
		t.RegisterPECreateResourceRelease,
		t.RegisterPEGetResourceRelease,

		// Project releases (admin: list/get/create; delete lives on the deployment toolset).
		t.RegisterPEListProjectReleases,
		t.RegisterPECreateProjectRelease,
		t.RegisterPEGetProjectRelease,

		// Plane resources (scope-collapsed: pass scope="cluster" for cluster-scoped planes).
		t.RegisterListDataPlanes,
		t.RegisterGetDataPlane,
		t.RegisterListWorkflowPlanes,
		t.RegisterGetWorkflowPlane,
		t.RegisterListObservabilityPlanes,
		t.RegisterGetObservabilityPlane,

		// Platform standards (scope-collapsed: pass scope="cluster" for the platform-wide resource).
		t.RegisterPEListComponentTypes,
		t.RegisterPEGetComponentType,
		t.RegisterPEGetComponentTypeSchema,
		t.RegisterPEListTraits,
		t.RegisterPEGetTrait,
		t.RegisterPEGetTraitSchema,
		t.RegisterPEListWorkflows,
		t.RegisterPEGetWorkflow,
		t.RegisterPEGetWorkflowSchema,
		t.RegisterPEListResourceTypes,
		t.RegisterPEGetResourceType,
		t.RegisterPEGetResourceTypeSchema,
		t.RegisterGetComponentTypeCreationSchema,
		t.RegisterGetTraitCreationSchema,
		t.RegisterGetWorkflowCreationSchema,
		t.RegisterGetResourceTypeCreationSchema,
		t.RegisterCreateComponentType,
		t.RegisterUpdateComponentType,
		t.RegisterDeleteComponentType,
		t.RegisterCreateTrait,
		t.RegisterUpdateTrait,
		t.RegisterDeleteTrait,
		t.RegisterPECreateWorkflow,
		t.RegisterPEUpdateWorkflow,
		t.RegisterPEDeleteWorkflow,
		t.RegisterCreateResourceType,
		t.RegisterUpdateResourceType,
		t.RegisterDeleteResourceType,

		// Project types (scope-collapsed: pass scope="cluster" for a ClusterProjectType).
		// Reads are dual-registered with the project toolset; writes are PE-only.
		t.RegisterPEListProjectTypes,
		t.RegisterPEGetProjectType,
		t.RegisterPEGetProjectTypeSchema,
		t.RegisterGetProjectTypeCreationSchema,
		t.RegisterCreateProjectType,
		t.RegisterUpdateProjectType,
		t.RegisterDeleteProjectType,

		// Authz roles (scope-collapsed)
		t.RegisterListAuthzRoles,
		t.RegisterGetAuthzRole,
		t.RegisterGetAuthzRoleCreationSchema,
		t.RegisterCreateAuthzRole,
		t.RegisterUpdateAuthzRole,
		t.RegisterDeleteAuthzRole,

		// Authz role bindings (scope-collapsed)
		t.RegisterListAuthzRoleBindings,
		t.RegisterGetAuthzRoleBinding,
		t.RegisterGetAuthzRoleBindingCreationSchema,
		t.RegisterCreateAuthzRoleBinding,
		t.RegisterUpdateAuthzRoleBinding,
		t.RegisterDeleteAuthzRoleBinding,

		// Secret references (list/get also registered by the namespace toolset)
		t.RegisterPEListSecretReferences,
		t.RegisterPEGetSecretReference,
		t.RegisterPECreateSecretReference,
		t.RegisterPEUpdateSecretReference,
		t.RegisterPEDeleteSecretReference,

		// Diagnostics
		t.RegisterGetResourceTree,
		t.RegisterGetResourceEvents,
		t.RegisterGetResourceLogs,
		t.RegisterEvaluateAuthz,
		t.RegisterListAuthzActions,
	}
}

// resourceToolRegistrations returns the dev-facing resource toolset.
// Mirrors componentToolRegistrations' role: Resource CRUD plus read-only access to
// (Cluster)ResourceType templates. Template writes, releases, and bindings live on
// the pe / deployment toolsets.
func (t *Toolsets) resourceToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		// Resource CRUD.
		t.RegisterListResources,
		t.RegisterGetResource,
		t.RegisterCreateResource,
		t.RegisterUpdateResource,
		t.RegisterDeleteResource,

		// Resource types (read-only, scope-collapsed: pass scope="cluster" for ClusterResourceType).
		t.RegisterListResourceTypes,
		t.RegisterGetResourceType,
		t.RegisterGetResourceTypeSchema,
	}
}

// Register registers all enabled tools with the MCP server and returns:
//   - perms: maps each registered tool name to its required authz action.
//     Each RegisterFunc declares its required action by writing to a perms map,
//     so this is always consistent with the set of registered tools.
//   - toolToToolsets: maps each registered tool name to the set of toolsets
//     that contain it. A tool can belong to more than one toolset (for example,
//     `list_component_types` is registered by both the component and pe
//     toolsets); this index records every toolset it appears in.
func (t *Toolsets) Register(s *mcp.Server) (
	perms map[string]ToolPermission,
	toolToToolsets map[string]map[ToolsetType]bool,
) {
	perms = make(map[string]ToolPermission)
	toolToToolsets = make(map[string]map[ToolsetType]bool)

	registerGroup := func(toolset ToolsetType, regs []RegisterFunc) {
		for _, registerFunc := range regs {
			// Use a fresh map per RegisterFunc so we can identify exactly
			// which tools it registered, even when multiple RegisterFuncs
			// share a tool name across toolsets.
			local := make(map[string]ToolPermission)
			registerFunc(s, local)
			for name, perm := range local {
				perms[name] = perm
				if toolToToolsets[name] == nil {
					toolToToolsets[name] = make(map[ToolsetType]bool)
				}
				toolToToolsets[name][toolset] = true
			}
		}
	}

	if t.NamespaceToolset != nil {
		registerGroup(ToolsetNamespace, t.namespaceToolRegistrations())
	}

	if t.ProjectToolset != nil {
		registerGroup(ToolsetProject, t.projectToolRegistrations())
	}

	if t.ComponentToolset != nil {
		registerGroup(ToolsetComponent, t.componentToolRegistrations())
	}

	if t.DeploymentToolset != nil {
		registerGroup(ToolsetDeployment, t.deploymentToolRegistrations())
	}

	if t.BuildToolset != nil {
		registerGroup(ToolsetBuild, t.buildToolRegistrations())
	}

	if t.PEToolset != nil {
		registerGroup(ToolsetPE, t.peToolRegistrations())
	}

	if t.ResourceToolset != nil {
		registerGroup(ToolsetResource, t.resourceToolRegistrations())
	}

	return perms, toolToToolsets
}
