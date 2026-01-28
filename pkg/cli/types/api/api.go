// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package api

// CommandImplementationInterface combines all APIs
type CommandImplementationInterface interface {
	NamespaceAPI
	ProjectAPI
	ComponentAPI
	ApplyAPI
	DeleteAPI
	LoginAPI
	LogoutAPI
	EnvironmentAPI
	DataPlaneAPI
	ConfigContextAPI
	DeploymentPipelineAPI
	WorkloadAPI
	ScaffoldAPI
	ComponentReleaseAPI
	ReleaseBindingAPI
	ListAPI
}

// NamespaceAPI defines namespace-related operations
type NamespaceAPI interface {
	CreateNamespace(params CreateNamespaceParams) error
}

// ProjectAPI defines project-related operations
type ProjectAPI interface {
	CreateProject(params CreateProjectParams) error
}

// ComponentAPI defines component-related operations
type ComponentAPI interface {
	CreateComponent(params CreateComponentParams) error
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

type EnvironmentAPI interface {
	CreateEnvironment(params CreateEnvironmentParams) error
}

type DataPlaneAPI interface {
	CreateDataPlane(params CreateDataPlaneParams) error
}

type ConfigContextAPI interface {
	GetContexts() error
	GetCurrentContext() error
	UseContext(params UseContextParams) error
	SetContext(params SetContextParams) error
	SetControlPlane(params SetControlPlaneParams) error
}

type DeploymentPipelineAPI interface {
	CreateDeploymentPipeline(params CreateDeploymentPipelineParams) error
}

// WorkloadAPI defines methods for creating workloads from descriptors
type WorkloadAPI interface {
	CreateWorkload(params CreateWorkloadParams) error
}

// ScaffoldAPI defines methods for scaffolding resources
type ScaffoldAPI interface {
	ScaffoldComponent(params ScaffoldComponentParams) error
}

// ComponentReleaseAPI defines component release operations (file-system mode)
type ComponentReleaseAPI interface {
	GenerateComponentRelease(params GenerateComponentReleaseParams) error
}

// ReleaseBindingAPI defines release binding operations (file-system mode)
type ReleaseBindingAPI interface {
	GenerateReleaseBinding(params GenerateReleaseBindingParams) error
}

// ListAPI defines methods for listing resources
type ListAPI interface {
	ListNamespaces(params ListNamespacesParams) error
	ListProjects(params ListProjectsParams) error
	ListComponents(params ListComponentsParams) error
	ListEnvironments(params ListEnvironmentsParams) error
	ListDataPlanes(params ListDataPlanesParams) error
	ListBuildPlanes(params ListBuildPlanesParams) error
	ListObservabilityPlanes(params ListObservabilityPlanesParams) error
	ListComponentTypes(params ListComponentTypesParams) error
	ListTraits(params ListTraitsParams) error
	ListWorkflows(params ListWorkflowsParams) error
	ListComponentWorkflows(params ListComponentWorkflowsParams) error
	ListSecretReferences(params ListSecretReferencesParams) error
}
