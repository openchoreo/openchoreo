// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package occ

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/apply"
	componentrelease "github.com/openchoreo/openchoreo/internal/occ/cmd/component-release"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/component"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/dataplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/deploymentpipeline"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/environment"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/namespace"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/project"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/workload"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/delete"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/list"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/logout"
	releasebinding "github.com/openchoreo/openchoreo/internal/occ/cmd/release-binding"
	scaffoldcomponent "github.com/openchoreo/openchoreo/internal/occ/cmd/scaffold/component"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CommandImplementation struct{}

var _ api.CommandImplementationInterface = &CommandImplementation{}

func NewCommandImplementation() *CommandImplementation {
	return &CommandImplementation{}
}

// Create Operations

func (c *CommandImplementation) CreateNamespace(params api.CreateNamespaceParams) error {
	orgImpl := namespace.NewCreateNamespaceImpl(constants.NamespaceV1Config)
	return orgImpl.CreateNamespace(params)
}

func (c *CommandImplementation) CreateProject(params api.CreateProjectParams) error {
	projImpl := project.NewCreateProjImpl(constants.ProjectV1Config)
	return projImpl.CreateProject(params)
}

func (c *CommandImplementation) CreateComponent(params api.CreateComponentParams) error {
	compImpl := component.NewCreateCompImpl(constants.ComponentV1Config)
	return compImpl.CreateComponent(params)
}

func (c *CommandImplementation) CreateDeployment(params api.CreateDeploymentParams) error {
	return fmt.Errorf("Deployment CRD has been removed")
}

func (c *CommandImplementation) CreateEnvironment(params api.CreateEnvironmentParams) error {
	envImpl := environment.NewCreateEnvironmentImpl(constants.EnvironmentV1Config)
	return envImpl.CreateEnvironment(params)
}

func (c *CommandImplementation) CreateDataPlane(params api.CreateDataPlaneParams) error {
	dpImpl := dataplane.NewCreateDataPlaneImpl(constants.DataPlaneV1Config)
	return dpImpl.CreateDataPlane(params)
}

func (c *CommandImplementation) CreateDeployableArtifact(params api.CreateDeployableArtifactParams) error {
	return fmt.Errorf("DeployableArtifact CRD has been removed")
}

func (c *CommandImplementation) CreateDeploymentPipeline(params api.CreateDeploymentPipelineParams) error {
	dpImpl := deploymentpipeline.NewCreateDeploymentPipelineImpl(constants.DeploymentPipelineV1Config)
	return dpImpl.CreateDeploymentPipeline(params)
}

func (c *CommandImplementation) CreateWorkload(params api.CreateWorkloadParams) error {
	workloadImpl := workload.NewCreateWorkloadImpl(constants.WorkloadV1Config)
	return workloadImpl.CreateWorkload(params)
}

// Delete Operations

func (c *CommandImplementation) Delete(params api.DeleteParams) error {
	deleteImpl := delete.NewDeleteImpl()
	return deleteImpl.Delete(params)
}

// Authentication Operations

func (c *CommandImplementation) Login(params api.LoginParams) error {
	loginImpl := login.NewAuthImpl()
	return loginImpl.Login(params)
}

func (c *CommandImplementation) IsLoggedIn() bool {
	loginImpl := login.NewAuthImpl()
	return loginImpl.IsLoggedIn()
}

func (c *CommandImplementation) GetLoginPrompt() string {
	loginImpl := login.NewAuthImpl()
	return loginImpl.GetLoginPrompt()
}

func (c *CommandImplementation) Logout() error {
	logoutImpl := logout.NewLogoutImpl()
	return logoutImpl.Logout()
}

// Configuration Operations

func (c *CommandImplementation) Apply(params api.ApplyParams) error {
	applyImpl := apply.NewApplyImpl()
	return applyImpl.Apply(params)
}

// Config Context Operations

func (c *CommandImplementation) GetContexts() error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.GetContexts()
}

func (c *CommandImplementation) GetCurrentContext() error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.GetCurrentContext()
}

func (c *CommandImplementation) SetContext(params api.SetContextParams) error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.SetContext(params)
}

func (c *CommandImplementation) UseContext(params api.UseContextParams) error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.UseContext(params)
}

func (c *CommandImplementation) SetControlPlane(params api.SetControlPlaneParams) error {
	configContextImpl := config.NewConfigContextImpl()
	return configContextImpl.SetControlPlane(params)
}

// Scaffold Operations

func (c *CommandImplementation) ScaffoldComponent(params api.ScaffoldComponentParams) error {
	scaffoldImpl := scaffoldcomponent.NewScaffoldComponentImpl()
	return scaffoldImpl.ScaffoldComponent(params)
}

// Component Release Operations (File-System Mode)

func (c *CommandImplementation) GenerateComponentRelease(params api.GenerateComponentReleaseParams) error {
	releaseImpl := componentrelease.NewComponentReleaseImpl()
	return releaseImpl.GenerateComponentRelease(params)
}

// Release Binding Operations (File-System Mode)

func (c *CommandImplementation) GenerateReleaseBinding(params api.GenerateReleaseBindingParams) error {
	bindingImpl := releasebinding.NewReleaseBindingImpl()
	return bindingImpl.GenerateReleaseBinding(params)
}

// List Operations

func (c *CommandImplementation) ListNamespaces(params api.ListNamespacesParams) error {
	listImpl := list.NewListImpl()
	return listImpl.ListNamespaces(params)
}

func (c *CommandImplementation) ListProjects(params api.ListProjectsParams) error {
	listImpl := list.NewListImpl()
	return listImpl.ListProjects(params)
}

func (c *CommandImplementation) ListComponents(params api.ListComponentsParams) error {
	listImpl := list.NewListImpl()
	return listImpl.ListComponents(params)
}

func (c *CommandImplementation) ListEnvironments(params api.ListEnvironmentsParams) error {
	listImpl := list.NewListImpl()
	return listImpl.ListEnvironments(params)
}

func (c *CommandImplementation) ListDataPlanes(params api.ListDataPlanesParams) error {
	listImpl := list.NewListImpl()
	return listImpl.ListDataPlanes(params)
}

func (c *CommandImplementation) ListBuildPlanes(params api.ListBuildPlanesParams) error {
	listImpl := list.NewListImpl()
	return listImpl.ListBuildPlanes(params)
}

func (c *CommandImplementation) ListObservabilityPlanes(params api.ListObservabilityPlanesParams) error {
	listImpl := list.NewListImpl()
	return listImpl.ListObservabilityPlanes(params)
}

func (c *CommandImplementation) ListComponentTypes(params api.ListComponentTypesParams) error {
	listImpl := list.NewListImpl()
	return listImpl.ListComponentTypes(params)
}

func (c *CommandImplementation) ListTraits(params api.ListTraitsParams) error {
	listImpl := list.NewListImpl()
	return listImpl.ListTraits(params)
}

func (c *CommandImplementation) ListWorkflows(params api.ListWorkflowsParams) error {
	listImpl := list.NewListImpl()
	return listImpl.ListWorkflows(params)
}

func (c *CommandImplementation) ListComponentWorkflows(params api.ListComponentWorkflowsParams) error {
	listImpl := list.NewListImpl()
	return listImpl.ListComponentWorkflows(params)
}

func (c *CommandImplementation) ListSecretReferences(params api.ListSecretReferencesParams) error {
	listImpl := list.NewListImpl()
	return listImpl.ListSecretReferences(params)
}
