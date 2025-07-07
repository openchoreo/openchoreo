// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package labels provides constant definitions for Kubernetes labels used across the Choreo logging system.
// This centralizes label definitions to eliminate magic strings and improve maintainability.
package labels

// Kubernetes label keys used for log filtering and identification across all logging components.
// These labels are applied to Kubernetes resources and used by:
// - OpenSearch queries for log filtering
// - AWS CloudWatch log queries
// - Azure Log Analytics queries
// - Log enrichment processes
const (
	// ComponentID identifies the specific microservice/component
	ComponentID = "component-name"

	// EnvironmentID identifies the deployment environment (dev, test, staging, prod, etc.)
	EnvironmentID = "environment-name"

	// ProjectID identifies the project that groups multiple components
	ProjectID = "project-name"

	// Version is the human-readable version string (e.g., "v1.2.3")
	Version = "version"

	// VersionID is the unique deployment version identifier (UUID)
	VersionID = "version_id"

	// OrganizationUUID identifies the organization that owns the resources
	OrganizationUUID = "organization-name"

	// PipelineID identifies the CI/CD pipeline that deployed the component
	PipelineID = "pipeline-id"

	// RunID identifies the specific execution run of a pipeline
	RunID = "run_id"

	// WorkflowName identifies the build/deployment workflow
	WorkflowName = "workflow_name"
)

// OpenSearch field paths for querying Kubernetes labels in log documents
const (
	// KubernetesLabelsPrefix is the base path for all Kubernetes labels in OpenSearch documents
	KubernetesLabelsPrefix = "kubernetes.labels"

	// Full field paths for OpenSearch queries
	OSComponentID      = KubernetesLabelsPrefix + "." + ComponentID
	OSEnvironmentID    = KubernetesLabelsPrefix + "." + EnvironmentID
	OSProjectID        = KubernetesLabelsPrefix + "." + ProjectID
	OSVersion          = KubernetesLabelsPrefix + "." + Version
	OSVersionID        = KubernetesLabelsPrefix + "." + VersionID
	OSOrganizationUUID = KubernetesLabelsPrefix + "." + OrganizationUUID
	OSPipelineID       = KubernetesLabelsPrefix + "." + PipelineID
	OSRunID            = KubernetesLabelsPrefix + "." + RunID
	OSWorkflowName     = KubernetesLabelsPrefix + "." + WorkflowName
)

// RequiredLabels are the required labels that must be present on all Choreo components for proper log filtering
var RequiredLabels = []string{
	ComponentID,
	EnvironmentID,
	ProjectID,
}

// CICDLabels are the CI/CD related labels used for build and deployment log tracking
var CICDLabels = []string{
	PipelineID,
	RunID,
	WorkflowName,
}

// VersioningLabels are the versioning labels used for deployment tracking and rollback scenarios
var VersioningLabels = []string{
	Version,
	VersionID,
}
