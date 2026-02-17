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

// ObservabilityPlaneRefKind defines the kind of observability plane referenced
// +kubebuilder:validation:Enum=ObservabilityPlane;ClusterObservabilityPlane
type ObservabilityPlaneRefKind string

const (
	// ObservabilityPlaneRefKindObservabilityPlane references a namespace-scoped ObservabilityPlane
	ObservabilityPlaneRefKindObservabilityPlane ObservabilityPlaneRefKind = "ObservabilityPlane"

	// ObservabilityPlaneRefKindClusterObservabilityPlane references a cluster-scoped ClusterObservabilityPlane
	ObservabilityPlaneRefKindClusterObservabilityPlane ObservabilityPlaneRefKind = "ClusterObservabilityPlane"
)

// ObservabilityPlaneRef represents a reference to an ObservabilityPlane or ClusterObservabilityPlane
type ObservabilityPlaneRef struct {
	// Kind is the kind of observability plane (ObservabilityPlane or ClusterObservabilityPlane)
	// +required
	Kind ObservabilityPlaneRefKind `json:"kind"`

	// Name is the name of the observability plane resource
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	Name string `json:"name"`
}

// ClusterObservabilityPlaneRefKind defines the kind for cluster-scoped observability plane references.
// Only ClusterObservabilityPlane is allowed since cluster-scoped resources can only reference
// other cluster-scoped resources.
// +kubebuilder:validation:Enum=ClusterObservabilityPlane
type ClusterObservabilityPlaneRefKind string

const (
	// ClusterObservabilityPlaneRefKindClusterObservabilityPlane references a cluster-scoped ClusterObservabilityPlane
	ClusterObservabilityPlaneRefKindClusterObservabilityPlane ClusterObservabilityPlaneRefKind = "ClusterObservabilityPlane"
)

// ClusterObservabilityPlaneRef represents a reference to a ClusterObservabilityPlane.
// Used by cluster-scoped resources (ClusterDataPlane, ClusterBuildPlane) that can only
// reference cluster-scoped observability planes.
type ClusterObservabilityPlaneRef struct {
	// Kind is the kind of observability plane. Must be ClusterObservabilityPlane.
	// +required
	Kind ClusterObservabilityPlaneRefKind `json:"kind"`

	// Name is the name of the ClusterObservabilityPlane resource
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	Name string `json:"name"`
}

// ComponentTypeRefKind defines the kind of component type referenced by a ComponentTypeRef
// +kubebuilder:validation:Enum=ComponentType;ClusterComponentType
type ComponentTypeRefKind string

const (
	// ComponentTypeRefKindComponentType references a namespace-scoped ComponentType
	ComponentTypeRefKindComponentType ComponentTypeRefKind = "ComponentType"

	// ComponentTypeRefKindClusterComponentType references a cluster-scoped ClusterComponentType
	ComponentTypeRefKindClusterComponentType ComponentTypeRefKind = "ClusterComponentType"
)

// ComponentTypeRef represents a reference to a ComponentType or ClusterComponentType
type ComponentTypeRef struct {
	// Kind is the kind of component type (ComponentType or ClusterComponentType)
	// +optional
	// +kubebuilder:default=ComponentType
	Kind ComponentTypeRefKind `json:"kind,omitempty"`

	// Name is the component type reference in format: {workloadType}/{componentTypeName}
	// +required
	// +kubebuilder:validation:Pattern=`^(deployment|statefulset|cronjob|job|proxy)/[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
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
