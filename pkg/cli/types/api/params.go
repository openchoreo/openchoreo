// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package api

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
	ComponentType    string
	Namespace        string
	Project          string
	Description      string
	GitRepositoryURL string
	Image            string
	Tag              string
	Port             int
	Endpoint         string
}

// LoginParams defines parameters for login
type LoginParams struct {
	ClientCredentials bool // Flag to use client credentials flow
	ClientID          string
	ClientSecret      string
	CredentialName    string // Name to save credential as
}

type LogParams struct {
	Name        string
	Namespace   string
	Project     string
	Component   string
	Build       string
	Type        string
	Environment string
	Follow      bool
	TailLines   int64
	Deployment  string
}

// CreateDeployableArtifactParams defines parameters for creating a deployable artifact
type CreateDeployableArtifactParams struct {
	Name        string
	Namespace   string
	Project     string
	Component   string
	DisplayName string
	Description string
}

// GetDeployableArtifactParams defines parameters for listing deployable artifacts
type GetDeployableArtifactParams struct {
	// Standard resource filters
	Namespace string
	Project   string
	Component string

	// Artifact-specific filters
	Build       string
	DockerImage string

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
	Environment string
	ArtifactRef string

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
	DeployableArtifact string
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

// CreateWorkloadParams defines parameters for creating a workload from a descriptor
type CreateWorkloadParams struct {
	FilePath      string
	NamespaceName string
	ProjectName   string
	ComponentName string
	ImageURL      string
	OutputPath    string
	DryRun        bool
	Mode          string // Operational mode: "api-server" or "file-system"
	RootDir       string // Root directory path for file-system mode
}

// ComponentLogsParams defines parameters for fetching component logs
type ComponentLogsParams struct {
	Namespace   string
	Project     string
	Component   string
	Environment string
	Follow      bool
	Since       string // duration like "1h", "30m", "5m"
}

// StartComponentWorkflowRunParams defines parameters for starting a component workflow run
type StartComponentWorkflowRunParams struct {
	Namespace  string
	Project    string
	Component  string
	Commit     string   // Git commit SHA
	Parameters []string // --set key=value format
}
