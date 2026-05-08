// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ConnectionTarget identifies a specific endpoint on a target component to resolve.
type ConnectionTarget struct {
	// Namespace is the control plane namespace of the target component.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// Project is the name of the project that owns the target component.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Project string `json:"project"`

	// Component is the name of the target component.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Component string `json:"component"`

	// Endpoint is the name of the endpoint on the target component.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`

	// Visibility is the desired visibility level for resolving the endpoint URL.
	// +kubebuilder:validation:Required
	Visibility EndpointVisibility `json:"visibility"`

	// Environment is the resolved target environment name for this connection.
	// This matches the consumer's environment.
	// +optional
	Environment string `json:"environment,omitempty"`
}

// ResolvedConnection holds the resolved URL for a single connection.
type ResolvedConnection struct {
	// Namespace is the control plane namespace of the target component.
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// Project is the name of the project that owns the target component.
	// +kubebuilder:validation:MinLength=1
	Project string `json:"project"`

	// Component is the name of the target component.
	// +kubebuilder:validation:MinLength=1
	Component string `json:"component"`

	// Endpoint is the name of the endpoint on the target component.
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`

	// Visibility is the visibility level at which the endpoint was resolved.
	Visibility EndpointVisibility `json:"visibility"`

	// URL is the resolved endpoint URL.
	URL EndpointURL `json:"url"`
}

// PendingConnection represents a connection that could not be resolved.
type PendingConnection struct {
	// Namespace is the control plane namespace of the target component.
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// Project is the name of the project that owns the target component.
	// +kubebuilder:validation:MinLength=1
	Project string `json:"project"`

	// Component is the name of the target component.
	// +kubebuilder:validation:MinLength=1
	Component string `json:"component"`

	// Endpoint is the name of the endpoint on the target component.
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`

	// Reason describes why the connection could not be resolved.
	Reason string `json:"reason"`
}

// ResourceDependencyTarget identifies a project-bound Resource the workload depends on.
// Used as a field-index source for the reverse-watch from ResourceReleaseBinding to
// ReleaseBinding: when a provider's status.outputs change, every consumer ReleaseBinding
// whose targets include the matching (project, resourceName, environment) tuple is enqueued.
type ResourceDependencyTarget struct {
	// Namespace is the control plane namespace of the consuming ReleaseBinding.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// Project is the name of the project that owns the target Resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Project string `json:"project"`

	// ResourceName is the name of the target Resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ResourceName string `json:"resourceName"`

	// Environment is the consumer's environment, used to select the correct
	// ResourceReleaseBinding from the targets in this project.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Environment string `json:"environment"`
}

// PendingResourceDependency represents a resource dependency that could not be resolved.
// Surfaces the failure on kubectl describe so users can diagnose missing bindings, missing
// outputs, or unready providers without inspecting controller logs.
type PendingResourceDependency struct {
	// Namespace is the control plane namespace of the consuming ReleaseBinding.
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// Project is the name of the project that owns the target Resource.
	// +kubebuilder:validation:MinLength=1
	Project string `json:"project"`

	// ResourceName is the name of the target Resource.
	// +kubebuilder:validation:MinLength=1
	ResourceName string `json:"resourceName"`

	// Reason describes why the dependency could not be resolved (binding not found,
	// output missing, provider not ready, etc.).
	Reason string `json:"reason"`
}

// ContainerOverride represents a single container in the workload.
type ContainerOverride struct {
	// Explicit environment variables.
	// +optional
	Env []EnvVar `json:"env,omitempty"`

	// File configurations.
	// +optional
	Files []FileVar `json:"files,omitempty"`
}

// WorkloadOverrideTemplateSpec defines overrides for workload configuration.
type WorkloadOverrideTemplateSpec struct {
	// Container override for env and file configurations.
	// +optional
	Container *ContainerOverride `json:"container,omitempty"`
}

// ReleaseBindingSpec defines the desired state of ReleaseBinding.
type ReleaseBindingSpec struct {
	// Owner identifies the component and project this ReleaseBinding belongs to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.owner is immutable"
	Owner ReleaseBindingOwner `json:"owner"`

	// EnvironmentName is the name of the environment this binds the ComponentRelease to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.environment is immutable"
	Environment string `json:"environment"`

	// ReleaseName is the name of the ComponentRelease to bind
	// When ComponentSpec.AutoDeploy is enabled, this field will be handled by the controller
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

	// ComponentTypeEnvironmentConfigs for ComponentType environmentConfigs parameters
	// These values override the defaults defined in the Component for this specific environment
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	ComponentTypeEnvironmentConfigs *runtime.RawExtension `json:"componentTypeEnvironmentConfigs,omitempty"`

	// TraitEnvironmentConfigs provides environment-specific overrides for trait configurations
	// Keyed by instanceName (which must be unique across all traits in the component)
	// Structure: map[instanceName]overrideValues
	// +optional
	TraitEnvironmentConfigs map[string]runtime.RawExtension `json:"traitEnvironmentConfigs,omitempty"`

	// WorkloadOverrides provides environment-specific overrides for the entire workload spec
	// These values override the workload specification for this specific environment
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	WorkloadOverrides *WorkloadOverrideTemplateSpec `json:"workloadOverrides,omitempty"`

	// State controls the state of the Release created by this binding.
	// Active: Resources are deployed normally
	// Undeploy: Resources are removed from the data plane
	// +kubebuilder:default=Active
	// +kubebuilder:validation:Enum=Active;Undeploy
	// +optional
	State ReleaseState `json:"state,omitempty"`
}

// ReleaseBindingOwner identifies the component this ReleaseBinding belongs to
type ReleaseBindingOwner struct {
	// ProjectName is the name of the project that owns this component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// ComponentName is the name of the component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// EndpointURL represents a structured URL with its components.
type EndpointURL struct {
	// Scheme is the URL scheme (e.g., http, https, tcp, udp, ws, wss, tls).
	// +optional
	Scheme string `json:"scheme,omitempty"`

	// Host is the hostname or IP address.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// Port is the port number.
	// +optional
	Port int32 `json:"port,omitempty"`

	// Path is the URL path.
	// +optional
	Path string `json:"path,omitempty"`
}

// EndpointGatewayURLs holds resolved gateway URLs for an endpoint.
type EndpointGatewayURLs struct {
	// HTTP is the HTTP gateway URL.
	// +optional
	HTTP *EndpointURL `json:"http,omitempty"`

	// HTTPS is the HTTPS gateway URL.
	// +optional
	HTTPS *EndpointURL `json:"https,omitempty"`

	// TLS is the TLS gateway URL.
	// +optional
	TLS *EndpointURL `json:"tls,omitempty"`
}

// EndpointURLStatus holds the resolved URLs for a single named workload endpoint.
type EndpointURLStatus struct {
	// Name is the endpoint name as defined in the Workload spec.
	Name string `json:"name"`

	// Type is the endpoint type (HTTP, gRPC, GraphQL, Websocket, TCP, UDP).
	// +optional
	Type EndpointType `json:"type,omitempty"`

	// ServiceURL is the in-cluster service URL for this endpoint.
	// +optional
	ServiceURL *EndpointURL `json:"serviceURL,omitempty"`

	// InvokeURL is the resolved public URL for this endpoint, derived from the
	// rendered HTTPRoute whose backendRef port matches the endpoint port.
	// +optional
	InvokeURL string `json:"invokeURL,omitempty"`

	// InternalURLs holds the resolved internal gateway URLs.
	// +optional
	InternalURLs *EndpointGatewayURLs `json:"internalURLs,omitempty"`

	// ExternalURLs holds the resolved external gateway URLs.
	// +optional
	ExternalURLs *EndpointGatewayURLs `json:"externalURLs,omitempty"`
}

// ReleaseBindingStatus defines the observed state of ReleaseBinding.
type ReleaseBindingStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastSpecUpdateTime is the timestamp of the last spec change observed by the controller.
	// Updated when the controller detects a new generation (i.e., spec was modified).
	// +optional
	LastSpecUpdateTime *metav1.Time `json:"lastSpecUpdateTime,omitempty"`

	// Conditions represent the latest available observations of the ReleaseBinding's current state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Endpoints contains the resolved invoke URLs for each named workload endpoint,
	// keyed by endpoint name. Populated once the component is deployed and the
	// corresponding HTTPRoutes are available.
	// +optional
	Endpoints []EndpointURLStatus `json:"endpoints,omitempty"`

	// ConnectionTargets lists the connection targets derived from the workload connections.
	// Used as an index source for finding consumer ReleaseBindings when a provider's endpoints change.
	// +optional
	ConnectionTargets []ConnectionTarget `json:"connectionTargets,omitempty"`

	// ResolvedConnections contains the connections that have been successfully resolved.
	// +optional
	ResolvedConnections []ResolvedConnection `json:"resolvedConnections,omitempty"`

	// PendingConnections contains the connections that could not be resolved.
	// +optional
	PendingConnections []PendingConnection `json:"pendingConnections,omitempty"`

	// ResourceDependencyTargets lists the resource dependency targets derived from the
	// workload's dependencies.resources[]. Used as an index source for finding consumer
	// ReleaseBindings when a provider ResourceReleaseBinding's status.outputs change.
	// +optional
	ResourceDependencyTargets []ResourceDependencyTarget `json:"resourceDependencyTargets,omitempty"`

	// PendingResourceDependencies contains the resource dependencies that could not be resolved.
	// +optional
	PendingResourceDependencies []PendingResourceDependency `json:"pendingResourceDependencies,omitempty"`

	// SecretReferenceNames lists the names of SecretReferences used by this ReleaseBinding's workload.
	// Used as an index source for finding affected ReleaseBindings when a SecretReference changes.
	// +optional
	SecretReferenceNames []string `json:"secretReferenceNames,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="Component",type=string,JSONPath=`.spec.owner.componentName`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ReleaseBinding is the Schema for the releasebindings API.
type ReleaseBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReleaseBindingSpec   `json:"spec,omitempty"`
	Status ReleaseBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ReleaseBindingList contains a list of ReleaseBinding.
type ReleaseBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReleaseBinding `json:"items"`
}

// GetConditions returns the conditions from the status
func (r *ReleaseBinding) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

// SetConditions sets the conditions in the status
func (r *ReleaseBinding) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&ReleaseBinding{}, &ReleaseBindingList{})
}
