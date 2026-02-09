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
	BuildPlaneAPI
	ObservabilityPlaneAPI
	ConfigContextAPI
	WorkloadAPI
	ComponentTypeAPI
	TraitAPI
	SecretReferenceAPI
	ComponentReleaseAPI
	ReleaseBindingAPI
	WorkflowAPI
	ComponentWorkflowAPI
	ComponentWorkflowRunAPI
	WorkflowRunAPI
}

// NamespaceAPI defines namespace-related operations
type NamespaceAPI interface {
	ListNamespaces(params ListNamespacesParams) error
}

// ProjectAPI defines project-related operations
type ProjectAPI interface {
	ListProjects(params ListProjectsParams) error
}

// ComponentAPI defines component-related operations
type ComponentAPI interface {
	ListComponents(params ListComponentsParams) error
	ScaffoldComponent(params ScaffoldComponentParams) error
	DeployComponent(params DeployComponentParams) error
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
	ListEnvironments(params ListEnvironmentsParams) error
}

type DataPlaneAPI interface {
	ListDataPlanes(params ListDataPlanesParams) error
}

type BuildPlaneAPI interface {
	ListBuildPlanes(params ListBuildPlanesParams) error
}

type ObservabilityPlaneAPI interface {
	ListObservabilityPlanes(params ListObservabilityPlanesParams) error
}

type ConfigContextAPI interface {
	AddContext(params AddContextParams) error
	ListContexts() error
	DeleteContext(params DeleteContextParams) error
	UpdateContext(params UpdateContextParams) error
	UseContext(params UseContextParams) error
	DescribeContext(params DescribeContextParams) error
	AddControlPlane(params AddControlPlaneParams) error
	ListControlPlanes() error
	UpdateControlPlane(params UpdateControlPlaneParams) error
	DeleteControlPlane(params DeleteControlPlaneParams) error
}

// WorkloadAPI defines methods for creating workloads from descriptors
type WorkloadAPI interface {
	CreateWorkload(params CreateWorkloadParams) error
}

// ComponentTypeAPI defines component type operations
type ComponentTypeAPI interface {
	ListComponentTypes(params ListComponentTypesParams) error
}

// TraitAPI defines trait operations
type TraitAPI interface {
	ListTraits(params ListTraitsParams) error
}

// SecretReferenceAPI defines secret reference operations
type SecretReferenceAPI interface {
	ListSecretReferences(params ListSecretReferencesParams) error
}

// ComponentReleaseAPI defines component release operations (file-system mode)
type ComponentReleaseAPI interface {
	GenerateComponentRelease(params GenerateComponentReleaseParams) error
	ListComponentReleases(params ListComponentReleasesParams) error
}

// ReleaseBindingAPI defines release binding operations (file-system mode)
type ReleaseBindingAPI interface {
	GenerateReleaseBinding(params GenerateReleaseBindingParams) error
	ListReleaseBindings(params ListReleaseBindingsParams) error
}

// WorkflowRunAPI defines methods for starting workflow runs
type WorkflowRunAPI interface {
	StartWorkflowRun(params StartWorkflowRunParams) error
	ListWorkflowRuns(params ListWorkflowRunsParams) error
}

// WorkflowAPI defines workflow operations
type WorkflowAPI interface {
	ListWorkflows(params ListWorkflowsParams) error
}

type ComponentWorkflowAPI interface {
	ListComponentWorkflows(params ListComponentWorkflowsParams) error
	StartComponentWorkflowRun(params StartComponentWorkflowRunParams) error
}

// ComponentWorkflowRunAPI defines methods for starting component workflow runs
type ComponentWorkflowRunAPI interface {
	ListComponentWorkflowRuns(params ListComponentWorkflowRunsParams) error
}
