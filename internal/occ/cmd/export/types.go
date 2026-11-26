// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import "time"

const (
	clusterProjectTypeKind                     = "ClusterProjectType"
	clusterComponentTypeKind                   = "ClusterComponentType"
	clusterResourceTypeKind                    = "ClusterResourceType"
	clusterEndpointTypeKind                    = "ClusterEndpointType"
	clusterTraitKind                           = "ClusterTrait"
	clusterWorkflowKind                        = "ClusterWorkflow"
	clusterDataPlaneKind                       = "ClusterDataPlane"
	clusterWorkflowPlaneKind                   = "ClusterWorkflowPlane"
	clusterObservabilityPlaneKind              = "ClusterObservabilityPlane"
	clusterAuthzRoleKind                       = "ClusterAuthzRole"
	clusterAuthzRoleBindingKind                = "ClusterAuthzRoleBinding"
	namespaceKind                              = "Namespace"
	deploymentPipelineKind                     = "DeploymentPipeline"
	environmentKind                            = "Environment"
	dataPlaneKind                              = "DataPlane"
	workflowPlaneKind                          = "WorkflowPlane"
	observabilityPlaneKind                     = "ObservabilityPlane"
	projectTypeKind                            = "ProjectType"
	componentTypeKind                          = "ComponentType"
	resourceTypeKind                           = "ResourceType"
	endpointTypeKind                           = "EndpointType"
	traitKind                                  = "Trait"
	workflowKind                               = "Workflow"
	authzRoleKind                              = "AuthzRole"
	authzRoleBindingKind                       = "AuthzRoleBinding"
	observabilityAlertsNotificationChannelKind = "ObservabilityAlertsNotificationChannel"
	secretReferenceKind                        = "SecretReference"
	projectKind                                = "Project"
	projectReleaseKind                         = "ProjectRelease"
	projectReleaseBindingKind                  = "ProjectReleaseBinding"
	componentKind                              = "Component"
	componentReleaseKind                       = "ComponentRelease"
	workloadKind                               = "Workload"
	releaseLineKind                            = "ReleaseLine"
	releaseBindingKind                         = "ReleaseBinding"
	endpointKind                               = "Endpoint"
	endpointReleaseKind                        = "EndpointRelease"
	endpointReleaseBindingKind                 = "EndpointReleaseBinding"
	resourceKind                               = "Resource"
	resourceReleaseKind                        = "ResourceRelease"
	resourceReleaseBindingKind                 = "ResourceReleaseBinding"
)

// Resource associates an API object or filtered document with its export identity.
// Namespace is empty for cluster-scoped objects, including Namespace objects.
type Resource struct {
	Kind      string
	Namespace string
	Name      string
	Object    any
}

// FetchResult contains selected resources, preserved history paths, and warnings.
type FetchResult struct {
	Resources []Resource
	// PreservedPaths are retained without being created or updated.
	PreservedPaths map[string]struct{}
	Warnings       []string
	// ReleaseCutoff is inclusive, or nil when exporting all history.
	ReleaseCutoff *time.Time
}

// SerializedFile is one manifest and its path relative to the export directory.
type SerializedFile struct {
	Path string
	Data []byte
}

// FileCounts records committed changes, excluding preserved historical files.
type FileCounts struct {
	Created   int
	Updated   int
	Unchanged int
	Deleted   int
}

// ExportResult supplies the committed export and its summary to the command.
type ExportResult struct {
	OutputDir     string
	Files         []SerializedFile
	Counts        FileCounts
	ReleaseCutoff *time.Time
	Warnings      []string
}
