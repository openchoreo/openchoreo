// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import "encoding/json"

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

	// ReleaseStateUndeploy indicates the Release should be removed from the data plane.
	// The Release resource is deleted, triggering cleanup of all data plane resources.
	ReleaseStateUndeploy ReleaseState = "Undeploy"
)

// EntitlementClaim represents a claim-value pair for subject identification
// Used by AuthzRoleBinding and ClusterAuthzRoleBinding
type EntitlementClaim struct {
	// Claim is the JWT claim name (e.g., "groups", "sub", "email")
	// +required
	Claim string `json:"claim"`

	// Value is the entitlement value to match
	// +required
	Value string `json:"value"`
}

// RoleRefKind defines the kind of role referenced by a RoleRef
// +kubebuilder:validation:Enum=AuthzRole;ClusterAuthzRole
type RoleRefKind string

const (
	// RoleRefKindAuthzRole references a namespaced AuthzRole
	RoleRefKindAuthzRole RoleRefKind = "AuthzRole"

	// RoleRefKindClusterAuthzRole references a cluster-scoped ClusterAuthzRole
	RoleRefKindClusterAuthzRole RoleRefKind = "ClusterAuthzRole"
)

// RoleRef represents a reference to an AuthzRole or ClusterAuthzRole
// Used by AuthzRoleBinding and ClusterAuthzRoleBinding
type RoleRef struct {
	// Kind is the kind of role (AuthzRole or ClusterAuthzRole)
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

// WorkflowPlaneRefKind defines the kind of workflow plane referenced by a WorkflowPlaneRef
// +kubebuilder:validation:Enum=WorkflowPlane;ClusterWorkflowPlane
type WorkflowPlaneRefKind string

const (
	// WorkflowPlaneRefKindWorkflowPlane references a namespace-scoped WorkflowPlane
	WorkflowPlaneRefKindWorkflowPlane WorkflowPlaneRefKind = "WorkflowPlane"

	// WorkflowPlaneRefKindClusterWorkflowPlane references a cluster-scoped ClusterWorkflowPlane
	WorkflowPlaneRefKindClusterWorkflowPlane WorkflowPlaneRefKind = "ClusterWorkflowPlane"
)

// WorkflowPlaneRef represents a reference to a WorkflowPlane or ClusterWorkflowPlane
type WorkflowPlaneRef struct {
	// Kind is the kind of workflow plane (WorkflowPlane or ClusterWorkflowPlane)
	// +required
	Kind WorkflowPlaneRefKind `json:"kind"`

	// Name is the name of the workflow plane resource
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`
}

// ClusterWorkflowPlaneRefKind defines the kind for cluster-scoped workflow plane references.
// Only ClusterWorkflowPlane is allowed since cluster-scoped resources can only reference
// other cluster-scoped resources.
// +kubebuilder:validation:Enum=ClusterWorkflowPlane
type ClusterWorkflowPlaneRefKind string

const (
	// ClusterWorkflowPlaneRefKindClusterWorkflowPlane references a cluster-scoped ClusterWorkflowPlane
	ClusterWorkflowPlaneRefKindClusterWorkflowPlane ClusterWorkflowPlaneRefKind = "ClusterWorkflowPlane"
)

// ClusterWorkflowPlaneRef represents a reference to a ClusterWorkflowPlane.
// Used by cluster-scoped resources (ClusterWorkflow) that can only
// reference cluster-scoped workflow planes.
type ClusterWorkflowPlaneRef struct {
	// Kind is the kind of workflow plane. Must be ClusterWorkflowPlane.
	// +required
	Kind ClusterWorkflowPlaneRefKind `json:"kind"`

	// Name is the name of the ClusterWorkflowPlane resource
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
// Used by cluster-scoped resources (ClusterDataPlane, ClusterWorkflowPlane) that can only
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

// TraitRefKind defines the kind of trait referenced by a TraitRef
// +kubebuilder:validation:Enum=Trait;ClusterTrait
type TraitRefKind string

const (
	// TraitRefKindTrait references a namespace-scoped Trait
	TraitRefKindTrait TraitRefKind = "Trait"

	// TraitRefKindClusterTrait references a cluster-scoped ClusterTrait
	TraitRefKindClusterTrait TraitRefKind = "ClusterTrait"
)

// TraitRef represents a reference to a Trait or ClusterTrait
type TraitRef struct {
	// Kind is the kind of trait (Trait or ClusterTrait)
	// +optional
	// +kubebuilder:default=Trait
	Kind TraitRefKind `json:"kind,omitempty"`

	// Name is the name of the trait resource
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ClusterTraitRefKind defines the kind for cluster-scoped trait references.
// Only ClusterTrait is allowed since cluster-scoped resources can only reference
// other cluster-scoped resources.
// +kubebuilder:validation:Enum=ClusterTrait
type ClusterTraitRefKind string

const (
	// ClusterTraitRefKindClusterTrait references a cluster-scoped ClusterTrait
	ClusterTraitRefKindClusterTrait ClusterTraitRefKind = "ClusterTrait"
)

// ClusterTraitRef represents a reference to a ClusterTrait.
// Used by cluster-scoped resources (ClusterComponentType) that can only
// reference cluster-scoped traits.
type ClusterTraitRef struct {
	// Kind is the kind of trait. Must be ClusterTrait.
	// +required
	Kind ClusterTraitRefKind `json:"kind"`

	// Name is the name of the ClusterTrait resource
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// WorkflowRefKind defines the kind for workflow references.
// +kubebuilder:validation:Enum=Workflow;ClusterWorkflow
type WorkflowRefKind string

const (
	// WorkflowRefKindWorkflow references a namespace-scoped Workflow
	WorkflowRefKindWorkflow WorkflowRefKind = "Workflow"

	// WorkflowRefKindClusterWorkflow references a cluster-scoped ClusterWorkflow
	WorkflowRefKindClusterWorkflow WorkflowRefKind = "ClusterWorkflow"
)

// WorkflowRef represents a reference to a Workflow resource.
type WorkflowRef struct {
	// Kind is the kind of workflow (Workflow or ClusterWorkflow).
	// +optional
	// +kubebuilder:default=ClusterWorkflow
	Kind WorkflowRefKind `json:"kind,omitempty"`

	// Name is the name of the workflow resource
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ClusterWorkflowRefKind defines the kind for cluster-scoped workflow references.
// +kubebuilder:validation:Enum=ClusterWorkflow
type ClusterWorkflowRefKind string

const (
	// ClusterWorkflowRefKindClusterWorkflow references a cluster-scoped ClusterWorkflow
	ClusterWorkflowRefKindClusterWorkflow ClusterWorkflowRefKind = "ClusterWorkflow"
)

// ClusterWorkflowRef represents a reference to a ClusterWorkflow resource.
// Used by cluster-scoped resources (ClusterComponentType) that can only
// reference cluster-scoped workflows.
type ClusterWorkflowRef struct {
	// Kind is the kind of workflow. Must be ClusterWorkflow.
	// +required
	Kind ClusterWorkflowRefKind `json:"kind"`

	// Name is the name of the ClusterWorkflow resource
	// +required
	// +kubebuilder:validation:MinLength=1
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

// ResourceTypeRefKind defines the kind of resource type referenced by a ResourceTypeRef.
// +kubebuilder:validation:Enum=ResourceType;ClusterResourceType
type ResourceTypeRefKind string

const (
	// ResourceTypeRefKindResourceType references a namespace-scoped ResourceType.
	ResourceTypeRefKindResourceType ResourceTypeRefKind = "ResourceType"

	// ResourceTypeRefKindClusterResourceType references a cluster-scoped ClusterResourceType.
	ResourceTypeRefKindClusterResourceType ResourceTypeRefKind = "ClusterResourceType"
)

// ResourceTypeRef represents a reference to a ResourceType or ClusterResourceType.
type ResourceTypeRef struct {
	// Kind is the kind of resource type (ResourceType or ClusterResourceType).
	// +optional
	// +kubebuilder:default=ResourceType
	Kind ResourceTypeRefKind `json:"kind,omitempty"`

	// Name is the name of the ResourceType or ClusterResourceType to reference.
	// Must be a valid DNS-1123 label since it identifies a Kubernetes object.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`
}

// DeploymentPipelineRefKind defines the kind of deployment pipeline referenced by a DeploymentPipelineRef
// +kubebuilder:validation:Enum=DeploymentPipeline
type DeploymentPipelineRefKind string

const (
	// DeploymentPipelineRefKindDeploymentPipeline references a namespace-scoped DeploymentPipeline
	DeploymentPipelineRefKindDeploymentPipeline DeploymentPipelineRefKind = "DeploymentPipeline"
)

// DeploymentPipelineRef represents a reference to a DeploymentPipeline
type DeploymentPipelineRef struct {
	// Kind is the kind of deployment pipeline (DeploymentPipeline)
	// +optional
	// +kubebuilder:default=DeploymentPipeline
	Kind DeploymentPipelineRefKind `json:"kind,omitempty"`

	// Name is the name of the deployment pipeline resource
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`
}

// UnmarshalJSON implements json.Unmarshaler to support both legacy string format
// (e.g. "my-pipeline") and the current object format (e.g. {"name":"my-pipeline"}).
func (r *DeploymentPipelineRef) UnmarshalJSON(data []byte) error {
	// Legacy format: plain string containing just the pipeline name
	if len(data) > 0 && data[0] == '"' {
		var name string
		if err := json.Unmarshal(data, &name); err != nil {
			return err
		}
		r.Name = name
		r.Kind = DeploymentPipelineRefKindDeploymentPipeline
		return nil
	}
	// Current format: object with name and optional kind fields.
	// Use an alias to avoid infinite recursion.
	type Alias DeploymentPipelineRef
	return json.Unmarshal(data, (*Alias)(r))
}

// EnvironmentRefKind defines the kind of environment referenced
// +kubebuilder:validation:Enum=Environment
type EnvironmentRefKind string

const (
	// EnvironmentRefKindEnvironment references a namespace-scoped Environment
	EnvironmentRefKindEnvironment EnvironmentRefKind = "Environment"
)

// EnvironmentRef represents a reference to an Environment (or ClusterEnvironment in the future)
type EnvironmentRef struct {
	// Kind is the kind of environment (Environment)
	// +optional
	// +kubebuilder:default=Environment
	Kind EnvironmentRefKind `json:"kind,omitempty"`

	// Name is the name of the environment resource
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`
}

// EffectType defines whether to allow or deny access
// Used by AuthzRoleBinding and ClusterAuthzRoleBinding
// +kubebuilder:validation:Enum=allow;deny
type EffectType string

const (
	EffectAllow EffectType = "allow"
	EffectDeny  EffectType = "deny"
)

// AuthzCondition represents the conditions under which an action is allowed or denied in an authorization role.
type AuthzCondition struct {
	// Actions is the list of actions this condition applies to.
	// Supports exact match ("releasebinding:create") and wildcards ("releasebinding:*").
	// +required
	// +kubebuilder:validation:MinItems=1
	Actions []string `json:"actions"`

	// Expression is a CEL expression that must evaluate to true for the action to be permitted.
	// Examples:
	//   resource.environment in ["dev", "staging"]
	// +required
	// +kubebuilder:validation:MinLength=1
	Expression string `json:"expression"`
}

// SecretKeyRef references a specific key in a Kubernetes Secret.
type SecretKeyRef struct {
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// ConfigMapKeyRef references a specific key in a Kubernetes ConfigMap.
type ConfigMapKeyRef struct {
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// ResourceRetainPolicy controls what happens to provisioned data on the data plane
// when its owning Resource or ResourceBinding is deleted.
// +kubebuilder:validation:Enum=Delete;Retain
type ResourceRetainPolicy string

const (
	// ResourceRetainPolicyDelete cascades deletion of the underlying provisioned data.
	ResourceRetainPolicyDelete ResourceRetainPolicy = "Delete"
	// ResourceRetainPolicyRetain keeps the underlying provisioned data after deletion.
	ResourceRetainPolicyRetain ResourceRetainPolicy = "Retain"
)
