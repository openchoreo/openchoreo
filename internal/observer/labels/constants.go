// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package labels provides constant definitions for Kubernetes labels used across the Choreo logging system.
// This centralizes label definitions to eliminate magic strings and improve maintainability.
package labels

import "strings"

// ReplaceDots replaces dots with underscores in a string
// This is used to match Fluent-Bit's Replace_Dots behavior in the OpenSearch output plugin.
func ReplaceDots(s string) string {
	return strings.ReplaceAll(s, ".", "_")
}

// Kubernetes label keys used for log filtering and identification across all logging components.
// These labels are applied to Kubernetes resources and used by:
// - OpenSearch queries for log filtering
// - AWS CloudWatch log queries
// - Azure Log Analytics queries
// - Log enrichment processes
const (
	// ComponentID identifies the specific microservice/component
	ComponentID = "openchoreo.dev/component-uid"

	// EnvironmentID identifies the deployment environment (dev, test, staging, prod, etc.)
	EnvironmentID = "openchoreo.dev/environment-uid"

	// ProjectID identifies the project that groups multiple components
	ProjectID = "openchoreo.dev/project-uid"

	// Version is the human-readable version string (e.g., "v1.2.3")
	Version = "version"

	// VersionID is the unique deployment version identifier (UUID)
	VersionID = "version_id"

	// NamespaceName identifies the namespace that owns the resources
	NamespaceName = "namespace-name"

	// PipelineID identifies the CI/CD pipeline that deployed the component
	PipelineID = "pipeline-id"

	// RunID identifies the specific execution run of a pipeline
	RunID = "run_id"

	// WorkflowName identifies the build/deployment workflow
	WorkflowName = "workflow_name"

	// BuildID identifies the specific build instance
	BuildID = "build-name"

	// BuildUUID identifies the unique build identifier (UUID)
	BuildUUID = "uuid"

	// Target identifies the target log category (build, runtime, gateway)
	Target = "target"
)

// Target value constants for different log types
const (
	// TargetBuild identifies build logs
	TargetBuild = "build"

	// TargetRuntime identifies runtime logs
	TargetRuntime = "runtime"

	// TargetGateway identifies gateway logs
	TargetGateway = "gateway"
)

// Query parameter constants for log types
const (
	// QueryParamLogTypeBuild identifies build log queries
	QueryParamLogTypeBuild = "BUILD"

	// QueryParamLogTypeRuntime identifies runtime log queries
	QueryParamLogTypeRuntime = "RUNTIME"
)

// OpenSearch field paths for querying Kubernetes labels in log documents
const (
	// KubernetesLabelsPrefix is the base path for all Kubernetes labels in OpenSearch documents
	KubernetesPrefix        = "kubernetes"
	KubernetesLabelsPrefix  = KubernetesPrefix + ".labels"
	KubernetesPodName       = KubernetesPrefix + ".pod_name"
	KubernetesContainerName = KubernetesPrefix + ".container_name"
)

// OpenSearch field paths with dots replaced by underscores in label keys
var (
	OSComponentID   = KubernetesLabelsPrefix + "." + ReplaceDots(ComponentID)
	OSEnvironmentID = KubernetesLabelsPrefix + "." + ReplaceDots(EnvironmentID)
	OSProjectID     = KubernetesLabelsPrefix + "." + ReplaceDots(ProjectID)
	OSVersion       = KubernetesLabelsPrefix + "." + ReplaceDots(Version)
	OSVersionID     = KubernetesLabelsPrefix + "." + ReplaceDots(VersionID)
	OSNamespaceName = KubernetesLabelsPrefix + "." + ReplaceDots(NamespaceName)
	OSPipelineID    = KubernetesLabelsPrefix + "." + ReplaceDots(PipelineID)
	OSRunID         = KubernetesLabelsPrefix + "." + ReplaceDots(RunID)
	OSWorkflowName  = KubernetesLabelsPrefix + "." + ReplaceDots(WorkflowName)
	OSBuildID       = KubernetesLabelsPrefix + "." + ReplaceDots(BuildID)
	OSBuildUUID     = KubernetesLabelsPrefix + "." + ReplaceDots(BuildUUID)
	OSTarget        = KubernetesLabelsPrefix + "." + ReplaceDots(Target)
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
