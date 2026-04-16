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
	}
}

// projectToolRegistrations returns the list of project toolset registration functions
func (t *Toolsets) projectToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterListProjects,
		t.RegisterCreateProject,
	}
}

// componentToolRegistrations returns the list of component toolset registration functions
func (t *Toolsets) componentToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterCreateComponent,
		t.RegisterListComponents,
		t.RegisterGetComponent,
		t.RegisterPatchComponent,
		t.RegisterListWorkloads,
		t.RegisterGetWorkload,
		t.RegisterCreateWorkload,
		t.RegisterUpdateWorkload,
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
		t.RegisterUpdateReleaseBindingState,
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
		t.RegisterGetResourceEvents,
		t.RegisterGetResourceLogs,
	}
}

// Register registers all enabled tools with the MCP server and returns the
// permissions map built as a side effect of registration. Each RegisterFunc
// declares its required authz action by writing to the perms map, so the
// returned map is always consistent with the set of registered tools.
func (t *Toolsets) Register(s *mcp.Server) map[string]ToolPermission {
	perms := make(map[string]ToolPermission)

	if t.NamespaceToolset != nil {
		for _, registerFunc := range t.namespaceToolRegistrations() {
			registerFunc(s, perms)
		}
	}

	if t.ProjectToolset != nil {
		for _, registerFunc := range t.projectToolRegistrations() {
			registerFunc(s, perms)
		}
	}

	if t.ComponentToolset != nil {
		for _, registerFunc := range t.componentToolRegistrations() {
			registerFunc(s, perms)
		}
	}

	if t.DeploymentToolset != nil {
		for _, registerFunc := range t.deploymentToolRegistrations() {
			registerFunc(s, perms)
		}
	}

	if t.BuildToolset != nil {
		for _, registerFunc := range t.buildToolRegistrations() {
			registerFunc(s, perms)
		}
	}

	if t.PEToolset != nil {
		for _, registerFunc := range t.peToolRegistrations() {
			registerFunc(s, perms)
		}
	}

	return perms
}
