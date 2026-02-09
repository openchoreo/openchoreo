// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// This file contains common types shared across multiple OpenChoreo CRDs

// EndpointStatus represents the observed state of an endpoint.
// Used by ServiceBinding, WebApplicationBinding, and other binding types.
// TODO: chathurangas: organization endpoint = inter-project endpoint. This is not being used. Refactor later
type EndpointStatus struct {
	// Name is the endpoint identifier matching spec.endpoints
	Name string `json:"name"`

	// Type is the endpoint type (uses EndpointType from endpoint_types.go)
	Type EndpointType `json:"type"`

	// Project contains access info for project-level visibility
	// +optional
	Project *EndpointAccess `json:"project,omitempty"`

	// Organization contains access info for organization-level visibility
	// +optional
	Organization *EndpointAccess `json:"organization,omitempty"`

	// Public contains access info for public visibility
	// +optional
	Public *EndpointAccess `json:"public,omitempty"`
}

// EndpointAccess contains all the information needed to connect to an endpoint
type EndpointAccess struct {
	// Host is the hostname or service name
	Host string `json:"host"`

	// Port is the port number
	Port int32 `json:"port"`

	// Scheme is the connection scheme (http, https, grpc, tcp)
	// +optional
	Scheme string `json:"scheme,omitempty"`

	// BasePath is the base URL path (for HTTP-based endpoints)
	// +optional
	BasePath string `json:"basePath,omitempty"`

	// URI is the computed URI for connecting to the endpoint
	// This field is automatically generated from host, port, scheme, and basePath
	// Examples: https://api.example.com:8080/v1, grpc://service:5050, tcp://localhost:9000
	// +optional
	URI string `json:"uri,omitempty"`

	// TODO: Add TLS and other details if needed
}

// EndpointExposeLevel defines the visibility scope for endpoint access
type EndpointExposeLevel string

const (
	// EndpointExposeLevelProject restricts endpoint access to components within the same project
	EndpointExposeLevelProject EndpointExposeLevel = "Project"

	// EndpointExposeLevelOrganization allows endpoint access across all projects within the same organization
	EndpointExposeLevelOrganization EndpointExposeLevel = "Organization"

	// EndpointExposeLevelPublic exposes the endpoint publicly, accessible from outside the organization
	EndpointExposeLevelPublic EndpointExposeLevel = "Public"
)

// ReleaseState defines the desired state of the Release created by a binding
type ReleaseState string

const (
	// ReleaseStateActive indicates the Release should be actively deployed.
	// Resources are deployed normally to the data plane.
	ReleaseStateActive ReleaseState = "Active"

	// ReleaseStateSuspend indicates the Release should be suspended.
	// Resources are scaled to zero or paused but not deleted.
	// - Deployments/StatefulSets: replicas set to 0
	// - Jobs: spec.suspend set to true
	// - CronJobs: spec.suspend set to true
	// - HPA: minReplicas and maxReplicas set to 0
	ReleaseStateSuspend ReleaseState = "Suspend"

	// ReleaseStateUndeploy indicates the Release should be removed from the data plane.
	// The Release resource is deleted, triggering cleanup of all data plane resources.
	ReleaseStateUndeploy ReleaseState = "Undeploy"
)

// EntitlementClaim represents a claim-value pair for subject identification
// Used by AuthzRoleBinding and AuthzClusterRoleBinding
type EntitlementClaim struct {
	// Claim is the JWT claim name (e.g., "groups", "sub", "email")
	// +required
	Claim string `json:"claim"`

	// Value is the entitlement value to match
	// +required
	Value string `json:"value"`
}

// RoleRefKind defines the kind of role referenced by a RoleRef
// +kubebuilder:validation:Enum=AuthzRole;AuthzClusterRole
type RoleRefKind string

const (
	// RoleRefKindAuthzRole references a namespaced AuthzRole
	RoleRefKindAuthzRole RoleRefKind = "AuthzRole"

	// RoleRefKindAuthzClusterRole references a cluster-scoped AuthzClusterRole
	RoleRefKindAuthzClusterRole RoleRefKind = "AuthzClusterRole"
)

// RoleRef represents a reference to an AuthzRole or AuthzClusterRole
// Used by AuthzRoleBinding and AuthzClusterRoleBinding
type RoleRef struct {
	// Kind is the kind of role (AuthzRole or AuthzClusterRole)
	// For AuthzRoleBinding: AuthzRole must be in the same namespace
	// +required
	Kind RoleRefKind `json:"kind"`

	// Name is the name of the role
	// +required
	Name string `json:"name"`
}

// DataPlaneRefKind defines the kind of data plane referenced by a DataPlaneRef
// +kubebuilder:validation:Enum=DataPlane;ClusterDataPlane
type DataPlaneRefKind string

const (
	// DataPlaneRefKindDataPlane references a namespace-scoped DataPlane
	DataPlaneRefKindDataPlane DataPlaneRefKind = "DataPlane"

	// DataPlaneRefKindClusterDataPlane references a cluster-scoped ClusterDataPlane
	DataPlaneRefKindClusterDataPlane DataPlaneRefKind = "ClusterDataPlane"
)

// DataPlaneRef represents a reference to a DataPlane or ClusterDataPlane
type DataPlaneRef struct {
	// Kind is the kind of data plane (DataPlane or ClusterDataPlane)
	// +required
	Kind DataPlaneRefKind `json:"kind"`

	// Name is the name of the data plane resource
	// +required
	Name string `json:"name"`
}

// BuildPlaneRefKind defines the kind of build plane referenced by a BuildPlaneRef
// +kubebuilder:validation:Enum=BuildPlane;ClusterBuildPlane
type BuildPlaneRefKind string

const (
	// BuildPlaneRefKindBuildPlane references a namespace-scoped BuildPlane
	BuildPlaneRefKindBuildPlane BuildPlaneRefKind = "BuildPlane"

	// BuildPlaneRefKindClusterBuildPlane references a cluster-scoped ClusterBuildPlane
	BuildPlaneRefKindClusterBuildPlane BuildPlaneRefKind = "ClusterBuildPlane"
)

// BuildPlaneRef represents a reference to a BuildPlane or ClusterBuildPlane
type BuildPlaneRef struct {
	// Kind is the kind of build plane (BuildPlane or ClusterBuildPlane)
	// +required
	Kind BuildPlaneRefKind `json:"kind"`

	// Name is the name of the build plane resource
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`
}

// EffectType defines whether to allow or deny access
// Used by AuthzRoleBinding and AuthzClusterRoleBinding
// +kubebuilder:validation:Enum=allow;deny
type EffectType string

const (
	EffectAllow EffectType = "allow"
	EffectDeny  EffectType = "deny"
)
