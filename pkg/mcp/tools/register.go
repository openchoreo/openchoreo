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
		t.RegisterGetNamespace,
		t.RegisterCreateNamespace,
		t.RegisterListSecretReferences,
	}
}

// projectToolRegistrations returns the list of project toolset registration functions
func (t *Toolsets) projectToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterListProjects,
		t.RegisterGetProject,
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
		t.RegisterUpdateComponentBinding,
		t.RegisterGetComponentWorkloads,
		t.RegisterListComponentReleases,
		t.RegisterCreateComponentRelease,
		t.RegisterGetComponentRelease,
		t.RegisterGetComponentSchema,
		t.RegisterGetComponentReleaseSchema,
		t.RegisterListReleaseBindings,
		t.RegisterPatchReleaseBinding,
		t.RegisterDeployRelease,
		t.RegisterPromoteComponent,
		t.RegisterCreateWorkload,
		t.RegisterListComponentTraits,
		t.RegisterUpdateComponentTraits,
		t.RegisterGetEnvironmentRelease,
		t.RegisterListComponentWorkflows,
		t.RegisterGetComponentWorkflowSchema,
		t.RegisterTriggerComponentWorkflow,
		t.RegisterListComponentWorkflowRuns,
		t.RegisterUpdateComponentWorkflowSchema,
	}
}

// buildToolRegistrations returns the list of build toolset registration functions
func (t *Toolsets) buildToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterListBuildTemplates,
		t.RegisterTriggerBuild,
		t.RegisterListBuilds,
		t.RegisterGetBuildObserverURL,
		t.RegisterListBuildPlanes,
	}
}

// deploymentToolRegistrations returns the list of deployment toolset registration functions
func (t *Toolsets) deploymentToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterGetDeploymentPipeline,
		t.RegisterGetComponentObserverURL,
	}
}

// infrastructureToolRegistrations returns the list of infrastructure toolset registration functions
func (t *Toolsets) infrastructureToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterListEnvironments,
		t.RegisterGetEnvironments,
		t.RegisterCreateEnvironment,
		t.RegisterListDataPlanes,
		t.RegisterGetDataPlane,
		t.RegisterCreateDataPlane,
		t.RegisterListComponentTypes,
		t.RegisterGetComponentTypeSchema,
		t.RegisterListWorkflows,
		t.RegisterGetWorkflowSchema,
		t.RegisterListTraits,
		t.RegisterGetTraitSchema,
		t.RegisterListObservabilityPlanes,
		t.RegisterListComponentWorkflowsOrgLevel,
		t.RegisterGetComponentWorkflowSchemaOrgLevel,
		t.RegisterListClusterDataPlanes,
		t.RegisterGetClusterDataPlane,
		t.RegisterCreateClusterDataPlane,
		t.RegisterListClusterBuildPlanes,
		t.RegisterListClusterObservabilityPlanes,
		t.RegisterListClusterComponentTypes,
		t.RegisterGetClusterComponentType,
		t.RegisterGetClusterComponentTypeSchema,
		t.RegisterListClusterTraits,
		t.RegisterGetClusterTrait,
		t.RegisterGetClusterTraitSchema,
	}
}

// schemaToolRegistrations returns the list of schema toolset registration functions
func (t *Toolsets) schemaToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterExplainSchema,
	}
}

// resourceToolRegistrations returns the list of resource toolset registration functions
func (t *Toolsets) resourceToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterApplyResource,
		t.RegisterDeleteResource,
		t.RegisterGetResource,
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

	// Register build tools if BuildToolset is enabled
	if t.BuildToolset != nil {
		for _, registerFunc := range t.buildToolRegistrations() {
			registerFunc(s)
		}
	}

	// Register deployment tools if DeploymentToolset is enabled
	if t.DeploymentToolset != nil {
		for _, registerFunc := range t.deploymentToolRegistrations() {
			registerFunc(s)
		}
	}

	// Register infrastructure tools if InfrastructureToolset is enabled
	if t.InfrastructureToolset != nil {
		for _, registerFunc := range t.infrastructureToolRegistrations() {
			registerFunc(s)
		}
	}

	// Register schema tools if SchemaToolset is enabled
	if t.SchemaToolset != nil {
		for _, registerFunc := range t.schemaToolRegistrations() {
			registerFunc(s)
		}
	}

	// Register resource tools if ResourceToolset is enabled
	if t.ResourceToolset != nil {
		for _, registerFunc := range t.resourceToolRegistrations() {
			registerFunc(s)
		}
	}
}
