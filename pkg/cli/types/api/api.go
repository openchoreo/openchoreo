// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package api

// CommandImplementationInterface combines all APIs
type CommandImplementationInterface interface {
	OrganizationAPI
	ProjectAPI
	ComponentAPI
	BuildAPI
	DeployableArtifactAPI
	DeploymentAPI
	ApplyAPI
	DeleteAPI
	LoginAPI
	LogoutAPI
	LogAPI
	EnvironmentAPI
	DataPlaneAPI
	DeploymentTrackAPI
	EndpointAPI
	ConfigContextAPI
	DeploymentPipelineAPI
	ConfigurationGroupAPI
}

// OrganizationAPI defines organization-related operations
type OrganizationAPI interface {
	CreateOrganization(params CreateOrganizationParams) error
	GetOrganization(params GetParams) error
}

// ProjectAPI defines project-related operations
type ProjectAPI interface {
	CreateProject(params CreateProjectParams) error
	GetProject(params GetProjectParams) error
}

// ComponentAPI defines component-related operations
type ComponentAPI interface {
	CreateComponent(params CreateComponentParams) error
	GetComponent(params GetComponentParams) error
}

// BuildAPI defines methods for building configurations
type BuildAPI interface {
	CreateBuild(params CreateBuildParams) error
	GetBuild(params GetBuildParams) error
}

type DeployableArtifactAPI interface {
	CreateDeployableArtifact(params CreateDeployableArtifactParams) error
	GetDeployableArtifact(params GetDeployableArtifactParams) error
}

type DeploymentAPI interface {
	CreateDeployment(params CreateDeploymentParams) error
	GetDeployment(params GetDeploymentParams) error
}

// ApplyAPI defines methods for applying configurations
type ApplyAPI interface {
	Apply(params ApplyParams) error
}

// DeleteAPI defines methods for deleting resources from configuration files
type DeleteAPI interface {
	Delete(params DeleteParams) error
}

// LoginAPI defines methods for authentication
type LoginAPI interface {
	Login(params LoginParams) error
	IsLoggedIn() bool
	GetLoginPrompt() string
}

// LogoutAPI defines methods for ending sessions
type LogoutAPI interface {
	Logout() error
}

type LogAPI interface {
	GetLogs(params LogParams) error
}

type EnvironmentAPI interface {
	CreateEnvironment(params CreateEnvironmentParams) error
	GetEnvironment(params GetEnvironmentParams) error
}

type DataPlaneAPI interface {
	CreateDataPlane(params CreateDataPlaneParams) error
	GetDataPlane(params GetDataPlaneParams) error
}

type DeploymentTrackAPI interface {
	CreateDeploymentTrack(params CreateDeploymentTrackParams) error
	GetDeploymentTrack(params GetDeploymentTrackParams) error
}

type EndpointAPI interface {
	GetEndpoint(params GetEndpointParams) error
}

type ConfigContextAPI interface {
	GetContexts() error
	GetCurrentContext() error
	UseContext(params UseContextParams) error
	SetContext(params SetContextParams) error
}

type DeploymentPipelineAPI interface {
	CreateDeploymentPipeline(params CreateDeploymentPipelineParams) error
	GetDeploymentPipeline(params GetDeploymentPipelineParams) error
}

type ConfigurationGroupAPI interface {
	GetConfigurationGroup(params GetConfigurationGroupParams) error
}
