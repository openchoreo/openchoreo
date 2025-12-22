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
	Limit        int // Maximum number of resources to return (0 for all; default when omitted)
}

// GetProjectParams defines parameters for listing projects
type GetProjectParams struct {
	Organization string
	OutputFormat string
	Name         string
	Limit        int // Maximum number of resources to return (0 for all; default when omitted)
}

// GetComponentParams defines parameters for listing components
type GetComponentParams struct {
	Organization string
	Project      string
	OutputFormat string
	Name         string
	Limit        int // Maximum number of resources to return (0 for all; default when omitted)
}

// CreateOrganizationParams defines parameters for creating organizations
type CreateOrganizationParams struct {
	Name        string
	DisplayName string
	Description string
}

// CreateProjectParams defines parameters for creating projects
type CreateProjectParams struct {
	Organization       string
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
	Organization     string
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
	KubeconfigPath string
	Kubecontext    string
}

type LogParams struct {
	Name            string
	Organization    string
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
	Organization    string
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
	Organization    string
	Project         string
	Component       string
	DeploymentTrack string
	OutputFormat    string
	Name            string
	Limit           int // Maximum number of resources to return (0 for all; default when omitted)
}

// CreateDeployableArtifactParams defines parameters for creating a deployable artifact
type CreateDeployableArtifactParams struct {
	Name            string
	Organization    string
	Project         string
	Component       string
	DeploymentTrack string
	DisplayName     string
	Description     string
}

// GetDeployableArtifactParams defines parameters for listing deployable artifacts
type GetDeployableArtifactParams struct {
	// Standard resource filters
	Organization string
	Project      string
	Component    string

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
	Limit        int // Maximum number of resources to return (0 for all; default when omitted)
}

// GetDeploymentParams defines parameters for listing deployments
type GetDeploymentParams struct {
	// Standard resource filters
	Organization string
	Project      string
	Component    string

	// Deployment specific filters
	Environment     string
	DeploymentTrack string
	ArtifactRef     string

	// Display options
	OutputFormat string
	Name         string
	Limit        int // Maximum number of resources to return (0 for all; default when omitted)
}

// CreateDeploymentParams defines parameters for creating a deployment
type CreateDeploymentParams struct {
	Name               string
	Organization       string
	Project            string
	Component          string
	Environment        string
	DeploymentTrack    string
	DeployableArtifact string
}

// CreateDeploymentTrackParams defines parameters for creating a deployment track
type CreateDeploymentTrackParams struct {
	Name              string
	Organization      string
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
	Organization string
	Project      string
	Component    string
	OutputFormat string
	Name         string
	Limit        int // Maximum number of resources to return (0 for all; default when omitted)
}

// CreateEnvironmentParams defines parameters for creating an environment
type CreateEnvironmentParams struct {
	Name         string
	Organization string
	DisplayName  string
	Description  string
	DataPlaneRef string
	IsProduction bool
	DNSPrefix    string
}

// GetEnvironmentParams defines parameters for listing environments
type GetEnvironmentParams struct {
	Organization string
	OutputFormat string
	Name         string
	Limit        int // Maximum number of resources to return (0 for all; default when omitted)
}

// CreateDataPlaneParams defines parameters for creating a data plane
type CreateDataPlaneParams struct {
	Name                    string
	Organization            string
	DisplayName             string
	Description             string
	ClusterAgentClientCA    string
	PublicVirtualHost       string
	OrganizationVirtualHost string
}

// GetDataPlaneParams defines parameters for listing data planes
type GetDataPlaneParams struct {
	Organization string
	OutputFormat string
	Name         string
	Limit        int // Maximum number of resources to return (0 for all; default when omitted)
}

// GetEndpointParams defines parameters for listing endpoints
type GetEndpointParams struct {
	Organization string
	Project      string
	Component    string
	Environment  string
	OutputFormat string
	Name         string
	Limit        int // Maximum number of resources to return (0 for all; default when omitted)
}

type SetContextParams struct {
	Name              string
	Organization      string
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
	Organization     string
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
	Organization string
	OutputFormat string
	Limit        int // Maximum number of resources to return (0 for all; default when omitted)
}

type GetConfigurationGroupParams struct {
	Name         string
	Organization string
	OutputFormat string
	Limit        int // Maximum number of resources to return (0 for all; default when omitted)
}

// SetControlPlaneParams defines parameters for setting control plane configuration
type SetControlPlaneParams struct {
	Endpoint string
	Token    string
}

// CreateWorkloadParams defines parameters for creating a workload from a descriptor
type CreateWorkloadParams struct {
	FilePath         string
	OrganizationName string
	ProjectName      string
	ComponentName    string
	ImageURL         string
	OutputPath       string
}

// ScaffoldComponentParams defines parameters for scaffolding a component
type ScaffoldComponentParams struct {
	ComponentName string
	ComponentType string   // format: workloadType/componentTypeName
	Traits        []string // trait names
	WorkflowName  string
	Organization  string
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
	OutputPath    string // Optional: custom output directory
	DryRun        bool   // Preview without writing files
}
