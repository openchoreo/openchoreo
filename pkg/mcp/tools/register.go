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
		t.RegisterCreateSecretReference,
		t.RegisterUpdateSecretReference,
		t.RegisterDeleteSecretReference,
	}
}

// projectToolRegistrations returns the list of project toolset registration functions
func (t *Toolsets) projectToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterListProjects,
		t.RegisterCreateProject,
		t.RegisterDeleteProject,
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
		// Platform standards (read-only, namespace-scoped)
		t.RegisterListComponentTypes,
		t.RegisterGetComponentTypeSchema,
		t.RegisterListTraits,
		t.RegisterGetTraitSchema,
		// Platform standards (read-only, cluster-scoped)
		t.RegisterListClusterComponentTypes,
		t.RegisterGetClusterComponentType,
		t.RegisterGetClusterComponentTypeSchema,
		t.RegisterListClusterTraits,
		t.RegisterGetClusterTrait,
		t.RegisterGetClusterTraitSchema,
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
		t.RegisterListWorkflows,
		t.RegisterGetWorkflowSchema,
		t.RegisterListClusterWorkflows,
		t.RegisterGetClusterWorkflow,
		t.RegisterGetClusterWorkflowSchema,
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

		// DataPlane read
		t.RegisterListDataPlanes,
		t.RegisterGetDataPlane,

		// WorkflowPlane read
		t.RegisterListWorkflowPlanes,
		t.RegisterGetWorkflowPlane,

		// ObservabilityPlane read
		t.RegisterListObservabilityPlanes,
		t.RegisterGetObservabilityPlane,

		// Cluster-scoped plane read
		t.RegisterListClusterDataPlanes,
		t.RegisterGetClusterDataPlane,
		t.RegisterListClusterWorkflowPlanes,
		t.RegisterGetClusterWorkflowPlane,
		t.RegisterListClusterObservabilityPlanes,
		t.RegisterGetClusterObservabilityPlane,

		// Platform standards read (namespace-scoped)
		t.RegisterPEListComponentTypes,
		t.RegisterPEGetComponentType,
		t.RegisterPEGetComponentTypeSchema,
		t.RegisterPEListTraits,
		t.RegisterPEGetTrait,
		t.RegisterPEGetTraitSchema,
		t.RegisterPEListWorkflows,
		t.RegisterPEGetWorkflow,
		t.RegisterPEGetWorkflowSchema,

		// Platform standards creation schemas
		t.RegisterGetComponentTypeCreationSchema,
		t.RegisterGetClusterComponentTypeCreationSchema,
		t.RegisterGetTraitCreationSchema,
		t.RegisterGetClusterTraitCreationSchema,
		t.RegisterGetWorkflowCreationSchema,
		t.RegisterGetClusterWorkflowCreationSchema,

		// Platform standards write (namespace-scoped)
		t.RegisterCreateComponentType,
		t.RegisterUpdateComponentType,
		t.RegisterDeleteComponentType,
		t.RegisterCreateTrait,
		t.RegisterUpdateTrait,
		t.RegisterDeleteTrait,
		t.RegisterPECreateWorkflow,
		t.RegisterPEUpdateWorkflow,
		t.RegisterPEDeleteWorkflow,

		// Platform standards read (cluster-scoped)
		t.RegisterPEListClusterComponentTypes,
		t.RegisterPEGetClusterComponentType,
		t.RegisterPEGetClusterComponentTypeSchema,
		t.RegisterPEListClusterTraits,
		t.RegisterPEGetClusterTrait,
		t.RegisterPEGetClusterTraitSchema,

		// Platform standards write (cluster-scoped)
		t.RegisterCreateClusterComponentType,
		t.RegisterUpdateClusterComponentType,
		t.RegisterDeleteClusterComponentType,
		t.RegisterCreateClusterTrait,
		t.RegisterUpdateClusterTrait,
		t.RegisterDeleteClusterTrait,
		t.RegisterCreateClusterWorkflow,
		t.RegisterUpdateClusterWorkflow,
		t.RegisterDeleteClusterWorkflow,

		// Diagnostics
		t.RegisterGetResourceTree,
		t.RegisterGetResourceEvents,
		t.RegisterGetResourceLogs,
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

	return perms, toolToToolsets
}
