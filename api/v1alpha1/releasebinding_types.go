// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

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

	// ComponentTypeEnvOverrides for ComponentType envOverrides parameters
	// These values override the defaults defined in the Component for this specific environment
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	ComponentTypeEnvOverrides *runtime.RawExtension `json:"componentTypeEnvOverrides,omitempty"`

	// TraitOverrides provides environment-specific overrides for trait configurations
	// Keyed by instanceName (which must be unique across all traits in the component)
	// Structure: map[instanceName]overrideValues
	// +optional
	TraitOverrides map[string]runtime.RawExtension `json:"traitOverrides,omitempty"`

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

	// Type is the endpoint type (HTTP, REST, gRPC, GraphQL, Websocket, TCP, UDP).
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
	// Conditions represent the latest available observations of the ReleaseBinding's current state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Endpoints contains the resolved invoke URLs for each named workload endpoint.
	// Populated once the component is deployed and the corresponding HTTPRoutes are available.
	// +optional
	Endpoints []EndpointURLStatus `json:"endpoints,omitempty"`
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
