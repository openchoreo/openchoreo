// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package occ

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/apply"
	componentrelease "github.com/openchoreo/openchoreo/internal/occ/cmd/component-release"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/build"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/component"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/dataplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/deploymentpipeline"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/deploymenttrack"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/environment"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/organization"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/project"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/create/workload"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/delete"
	getbuild "github.com/openchoreo/openchoreo/internal/occ/cmd/get/build"
	getcomponent "github.com/openchoreo/openchoreo/internal/occ/cmd/get/component"
	getconfigurationgroup "github.com/openchoreo/openchoreo/internal/occ/cmd/get/configurationgroup"
	getdataplane "github.com/openchoreo/openchoreo/internal/occ/cmd/get/dataplane"
	getdeploymentpipeline "github.com/openchoreo/openchoreo/internal/occ/cmd/get/deploymentpipeline"
	getdeploymenttrack "github.com/openchoreo/openchoreo/internal/occ/cmd/get/deploymenttrack"
	getenv "github.com/openchoreo/openchoreo/internal/occ/cmd/get/environment"
	getorganization "github.com/openchoreo/openchoreo/internal/occ/cmd/get/organization"
	getproject "github.com/openchoreo/openchoreo/internal/occ/cmd/get/project"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/logout"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/logs"
	scaffoldcomponent "github.com/openchoreo/openchoreo/internal/occ/cmd/scaffold/component"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CommandImplementation struct{}

var _ api.CommandImplementationInterface = &CommandImplementation{}

func NewCommandImplementation() *CommandImplementation {
	return &CommandImplementation{}
}

// Get Operations

func (c *CommandImplementation) GetOrganization(params api.GetParams) error {
	orgImpl := getorganization.NewGetOrgImpl(constants.OrganizationV1Config)
	return orgImpl.GetOrganization(params)
}

func (c *CommandImplementation) GetProject(params api.GetProjectParams) error {
	projImpl := getproject.NewGetProjImpl(constants.ProjectV1Config)
	return projImpl.GetProject(params)
}

func (c *CommandImplementation) GetComponent(params api.GetComponentParams) error {
	compImpl := getcomponent.NewGetCompImpl(constants.ComponentV1Config)
	return compImpl.GetComponent(params)
}

func (c *CommandImplementation) GetBuild(params api.GetBuildParams) error {
	buildImpl := getbuild.NewGetBuildImpl(constants.BuildV1Config)
	return buildImpl.GetBuild(params)
}

func (c *CommandImplementation) GetDeployableArtifact(params api.GetDeployableArtifactParams) error {
	return fmt.Errorf("DeployableArtifact CRD has been removed")
}

func (c *CommandImplementation) GetDeployment(params api.GetDeploymentParams) error {
	return fmt.Errorf("Deployment CRD has been removed")
}

func (c *CommandImplementation) GetEnvironment(params api.GetEnvironmentParams) error {
	envImpl := getenv.NewGetEnvironmentImpl(constants.EnvironmentV1Config)
	return envImpl.GetEnvironment(params)
}

func (c *CommandImplementation) GetDataPlane(params api.GetDataPlaneParams) error {
	dpImpl := getdataplane.NewGetDataPlaneImpl(constants.DataPlaneV1Config)
	return dpImpl.GetDataPlane(params)
}

func (c *CommandImplementation) GetDeploymentTrack(params api.GetDeploymentTrackParams) error {
	trackImpl := getdeploymenttrack.NewGetDeploymentTrackImpl(constants.DeploymentTrackV1Config)
	return trackImpl.GetDeploymentTrack(params)
}

func (c *CommandImplementation) GetEndpoint(params api.GetEndpointParams) error {
	return fmt.Errorf("Endpoint CRD has been removed")
}

// Create Operations

func (c *CommandImplementation) CreateOrganization(params api.CreateOrganizationParams) error {
	orgImpl := organization.NewCreateOrgImpl(constants.OrganizationV1Config)
	return orgImpl.CreateOrganization(params)
}

func (c *CommandImplementation) CreateProject(params api.CreateProjectParams) error {
	projImpl := project.NewCreateProjImpl(constants.ProjectV1Config)
	return projImpl.CreateProject(params)
}

func (c *CommandImplementation) CreateComponent(params api.CreateComponentParams) error {
	compImpl := component.NewCreateCompImpl(constants.ComponentV1Config)
	return compImpl.CreateComponent(params)
}

func (c *CommandImplementation) CreateBuild(params api.CreateBuildParams) error {
	buildImpl := build.NewCreateBuildImpl(constants.ComponentV1Config)
	return buildImpl.CreateBuild(params)
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

func (c *CommandImplementation) CreateDeploymentTrack(params api.CreateDeploymentTrackParams) error {
	trackImpl := deploymenttrack.NewCreateDeploymentTrackImpl(constants.DeploymentTrackV1Config)
	return trackImpl.CreateDeploymentTrack(params)
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

// Logs Operations

func (c *CommandImplementation) GetLogs(params api.LogParams) error {
	logsImpl := logs.NewLogsImpl()
	return logsImpl.GetLogs(params)
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

func (c *CommandImplementation) GetDeploymentPipeline(params api.GetDeploymentPipelineParams) error {
	pipelineImpl := getdeploymentpipeline.NewGetDeploymentPipelineImpl(constants.DeploymentPipelineV1Config)
	return pipelineImpl.GetDeploymentPipeline(params)
}

func (c *CommandImplementation) GetConfigurationGroup(params api.GetConfigurationGroupParams) error {
	configurationGroupImpl := getconfigurationgroup.NewGetConfigurationGroupImpl(constants.ConfigurationGroupV1Config)
	return configurationGroupImpl.GetConfigurationGroup(params)
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
	// TODO: Implement this
	return fmt.Errorf("release-binding generate command is not yet implemented")
}
