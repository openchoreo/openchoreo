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
		t.RegisterGetComponentWorkloads,
		t.RegisterGetComponentWorkload,
		t.RegisterListComponentReleases,
		t.RegisterCreateComponentRelease,
		t.RegisterGetComponentRelease,
		t.RegisterGetComponentSchema,
		t.RegisterListReleaseBindings,
		t.RegisterGetReleaseBinding,
		t.RegisterPatchReleaseBinding,
		t.RegisterDeployRelease,
		t.RegisterPromoteComponent,
		t.RegisterCreateWorkload,
		t.RegisterGetEnvironmentRelease,
		t.RegisterUpdateReleaseBindingState,
		t.RegisterGetComponentReleaseSchema,
		t.RegisterTriggerWorkflowRun,
		t.RegisterListComponentTypes,
		t.RegisterGetComponentTypeSchema,
		t.RegisterListTraits,
		t.RegisterGetTraitSchema,
		t.RegisterCreateWorkflowRun,
		t.RegisterListWorkflowRuns,
		t.RegisterGetWorkflowRun,
		t.RegisterListClusterComponentTypes,
		t.RegisterGetClusterComponentType,
		t.RegisterGetClusterComponentTypeSchema,
		t.RegisterListClusterTraits,
		t.RegisterGetClusterTrait,
		t.RegisterGetClusterTraitSchema,
		t.RegisterListWorkflows,
		t.RegisterGetWorkflowSchema,
	}
}

// infrastructureToolRegistrations returns the list of infrastructure toolset registration functions
func (t *Toolsets) infrastructureToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterListEnvironments,
		t.RegisterGetEnvironments,
		t.RegisterGetDeploymentPipeline,
		t.RegisterListDeploymentPipelines,
		t.RegisterGetObserverURL,
	}
}

// peToolRegistrations returns the list of pe toolset registration functions
func (t *Toolsets) peToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterCreateEnvironment,
		t.RegisterListDataPlanes,
		t.RegisterGetDataPlane,
		t.RegisterCreateDataPlane,
		t.RegisterListObservabilityPlanes,
		t.RegisterListBuildPlanes,
		t.RegisterListClusterDataPlanes,
		t.RegisterGetClusterDataPlane,
		t.RegisterCreateClusterDataPlane,
		t.RegisterListClusterBuildPlanes,
		t.RegisterListClusterObservabilityPlanes,
	}
}

func (t *Toolsets) Register(s *mcp.Server) {
	// Register namespace tools if NamespaceToolset is enabled
	if t.NamespaceToolset != nil {
		for _, registerFunc := range t.namespaceToolRegistrations() {
			registerFunc(s)
		}
	}

	// Register project tools if ProjectToolset is enabled
	if t.ProjectToolset != nil {
		for _, registerFunc := range t.projectToolRegistrations() {
			registerFunc(s)
		}
	}

	// Register component tools if ComponentToolset is enabled
	if t.ComponentToolset != nil {
		for _, registerFunc := range t.componentToolRegistrations() {
			registerFunc(s)
		}
	}

	// Register infrastructure tools if InfrastructureToolset is enabled
	if t.InfrastructureToolset != nil {
		for _, registerFunc := range t.infrastructureToolRegistrations() {
			registerFunc(s)
		}
	}

	// Register platform engineering tools if PEToolset is enabled
	if t.PEToolset != nil {
		for _, registerFunc := range t.peToolRegistrations() {
			registerFunc(s)
		}
	}
}
