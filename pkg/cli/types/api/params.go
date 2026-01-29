// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// GetParams defines common parameters for listing resources
type GetParams struct {
	OutputFormat string
	Name         string
}

// GetProjectParams defines parameters for listing projects
type GetProjectParams struct {
	Namespace    string
	OutputFormat string
	Name         string
}

// GetComponentParams defines parameters for listing components
type GetComponentParams struct {
	Namespace    string
	Project      string
	OutputFormat string
	Name         string
}

// CreateNamespaceParams defines parameters for creating namespaces
type CreateNamespaceParams struct {
	Name        string
	DisplayName string
	Description string
}

// CreateProjectParams defines parameters for creating projects
type CreateProjectParams struct {
	Namespace          string
	Name               string
	DisplayName        string
	Description        string
	DeploymentPipeline string
}

// CreateComponentParams contains parameters for component creation
type CreateComponentParams struct {
	Name             string
	DisplayName      string
	Type             openchoreov1alpha1.DefinedComponentType
	Namespace        string
	Project          string
	Description      string
	GitRepositoryURL string
	Branch           string
	Path             string
	DockerContext    string
	DockerFile       string
	BuildpackName    string
	BuildpackVersion string
	BuildConfig      string
	Image            string
	Tag              string
	Port             int
	Endpoint         string
}

// ApplyParams defines parameters for applying configuration files
type ApplyParams struct {
	FilePath string
}

type DeleteParams struct {
	FilePath string
	Wait     bool
}

// LoginParams defines parameters for login
type LoginParams struct {
	ClientCredentials bool // Flag to use client credentials flow
	ClientID          string
	ClientSecret      string
	CredentialName    string // Name to save credential as
	URL               string // Control plane URL to update
}

type LogParams struct {
	Name            string
	Namespace       string
	Project         string
	Component       string
	Build           string
	Type            string
	Environment     string
	Follow          bool
	TailLines       int64
	Deployment      string
	DeploymentTrack string
}

// CreateBuildParams contains parameters for build creation
type CreateBuildParams struct {
	// Basic metadata
	Name            string
	Namespace       string
	Project         string
	Component       string
	DeploymentTrack string
	// Build configuration
	Docker    *openchoreov1alpha1.DockerConfiguration
	Buildpack *openchoreov1alpha1.BuildpackConfiguration
	// Build spec
	Branch    string
	Path      string
	Revision  string
	AutoBuild bool
}

// GetBuildParams defines parameters for listing builds
type GetBuildParams struct {
	Namespace       string
	Project         string
	Component       string
	DeploymentTrack string
	OutputFormat    string
	Name            string
}

// CreateDeployableArtifactParams defines parameters for creating a deployable artifact
type CreateDeployableArtifactParams struct {
	Name            string
	Namespace       string
	Project         string
	Component       string
	DeploymentTrack string
	DisplayName     string
	Description     string
}

// GetDeployableArtifactParams defines parameters for listing deployable artifacts
type GetDeployableArtifactParams struct {
	// Standard resource filters
	Namespace string
	Project   string
	Component string

	// Artifact-specific filters
	DeploymentTrack string
	Build           string
	DockerImage     string

	// Display options
	OutputFormat string
	Name         string

	// Optional filters
	GitRevision  string
	DisabledOnly bool
}

// GetDeploymentParams defines parameters for listing deployments
type GetDeploymentParams struct {
	// Standard resource filters
	Namespace string
	Project   string
	Component string

	// Deployment specific filters
	Environment     string
	DeploymentTrack string
	ArtifactRef     string

	// Display options
	OutputFormat string
	Name         string
}

// CreateDeploymentParams defines parameters for creating a deployment
type CreateDeploymentParams struct {
	Name               string
	Namespace          string
	Project            string
	Component          string
	Environment        string
	DeploymentTrack    string
	DeployableArtifact string
}

// CreateDeploymentTrackParams defines parameters for creating a deployment track
type CreateDeploymentTrackParams struct {
	Name              string
	Namespace         string
	Project           string
	Component         string
	DisplayName       string
	Description       string
	APIVersion        string
	AutoDeploy        bool
	BuildTemplateSpec *openchoreov1alpha1.BuildTemplateSpec
}

// GetDeploymentTrackParams defines parameters for listing deployment tracks
type GetDeploymentTrackParams struct {
	Namespace    string
	Project      string
	Component    string
	OutputFormat string
	Name         string
}

// CreateEnvironmentParams defines parameters for creating an environment
type CreateEnvironmentParams struct {
	Name         string
	Namespace    string
	DisplayName  string
	Description  string
	DataPlaneRef string
	IsProduction bool
	DNSPrefix    string
}

// GetEnvironmentParams defines parameters for listing environments
type GetEnvironmentParams struct {
	Namespace    string
	OutputFormat string
	Name         string
}

// CreateDataPlaneParams defines parameters for creating a data plane
type CreateDataPlaneParams struct {
	Name                 string
	Namespace            string
	DisplayName          string
	Description          string
	ClusterAgentClientCA string
	PublicVirtualHost    string
	NamespaceVirtualHost string
}

// GetDataPlaneParams defines parameters for listing data planes
type GetDataPlaneParams struct {
	Namespace    string
	OutputFormat string
	Name         string
}

// GetEndpointParams defines parameters for listing endpoints
type GetEndpointParams struct {
	Namespace    string
	Project      string
	Component    string
	Environment  string
	OutputFormat string
	Name         string
}

type SetContextParams struct {
	Name              string
	Namespace         string
	Project           string
	Component         string
	Environment       string
	DataPlane         string
	Mode              string // "api-server" or "file-system"
	RootDirectoryPath string // Path for file-system mode
}

type UseContextParams struct {
	Name string
}

type CreateDeploymentPipelineParams struct {
	Name             string
	DisplayName      string
	Description      string
	Namespace        string
	PromotionPaths   []PromotionPathParams
	EnvironmentOrder []string // Ordered list of environment names for promotion path
}

type PromotionPathParams struct {
	SourceEnvironment  string
	TargetEnvironments []TargetEnvironmentParams
}

type TargetEnvironmentParams struct {
	Name                     string
	RequiresApproval         bool
	IsManualApprovalRequired bool
}

type GetDeploymentPipelineParams struct {
	Name         string
	Namespace    string
	OutputFormat string
}

type GetConfigurationGroupParams struct {
	Name         string
	Namespace    string
	OutputFormat string
}

// SetControlPlaneParams defines parameters for setting control plane configuration
type SetControlPlaneParams struct {
	Name string
	URL  string
}

// CreateWorkloadParams defines parameters for creating a workload from a descriptor
type CreateWorkloadParams struct {
	FilePath      string
	NamespaceName string
	ProjectName   string
	ComponentName string
	ImageURL      string
	OutputPath    string
	DryRun        bool
}

// ScaffoldComponentParams defines parameters for scaffolding a component
type ScaffoldComponentParams struct {
	ComponentName string
	ComponentType string   // format: workloadType/componentTypeName
	Traits        []string // trait names
	WorkflowName  string
	Namespace     string
	ProjectName   string
	OutputPath    string
	SkipComments  bool // skip structural comments and field descriptions
	SkipOptional  bool // skip optional fields without defaults
}

// GenerateComponentReleaseParams defines parameters for generating component releases
type GenerateComponentReleaseParams struct {
	All           bool   // Generate for all components
	ProjectName   string // Generate for all components in this project
	ComponentName string // Generate for specific component
	ReleaseName   string // Optional: custom release name (only valid with --component)
	OutputPath    string // Optional: custom output directory
	DryRun        bool   // Preview without writing files
}

// GenerateReleaseBindingParams defines parameters for generating release bindings
type GenerateReleaseBindingParams struct {
	All              bool   // Generate for all components
	ProjectName      string // Generate for all components in this project
	ComponentName    string // Generate for specific component
	ComponentRelease string // Explicit component release name (only with project+component)
	TargetEnv        string // Required: target environment name
	UsePipeline      string // Required: deployment pipeline name
	OutputPath       string // Optional: custom output directory
	DryRun           bool   // Preview without writing files
}

// ListNamespacesParams defines parameters for listing namespaces
type ListNamespacesParams struct{}

// ListProjectsParams defines parameters for listing projects
type ListProjectsParams struct {
	Namespace string
}

// ListComponentsParams defines parameters for listing components
type ListComponentsParams struct {
	Namespace string
	Project   string
}

// ListEnvironmentsParams defines parameters for listing environments
type ListEnvironmentsParams struct {
	Namespace string
}

// ListDataPlanesParams defines parameters for listing data planes
type ListDataPlanesParams struct {
	Namespace string
}

// ListBuildPlanesParams defines parameters for listing build planes
type ListBuildPlanesParams struct {
	Namespace string
}

// ListObservabilityPlanesParams defines parameters for listing observability planes
type ListObservabilityPlanesParams struct {
	Namespace string
}

// ListComponentTypesParams defines parameters for listing component types
type ListComponentTypesParams struct {
	Namespace string
}

// ListTraitsParams defines parameters for listing traits
type ListTraitsParams struct {
	Namespace string
}

// ListWorkflowsParams defines parameters for listing workflows
type ListWorkflowsParams struct {
	Namespace string
}

// ListComponentWorkflowsParams defines parameters for listing component workflows
type ListComponentWorkflowsParams struct {
	Namespace string
}

// ListSecretReferencesParams defines parameters for listing secret references
type ListSecretReferencesParams struct {
	Namespace string
}
