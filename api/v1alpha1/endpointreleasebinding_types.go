// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EndpointReleaseBindingSpec defines the desired state of EndpointReleaseBinding.
// Pins an EndpointRelease to an Environment and carries per-env overrides.
// The Endpoint controller never creates or modifies EndpointReleaseBindings;
// they are authored externally (kubectl, GitOps, API server).
type EndpointReleaseBindingSpec struct {
	// Owner identifies the endpoint and project this EndpointReleaseBinding belongs to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.owner is immutable"
	Owner EndpointReleaseBindingOwner `json:"owner"`

	// Environment is the name of the Environment this binding targets.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.environment is immutable"
	Environment string `json:"environment"`

	// EndpointRelease is the name of the EndpointRelease pinned by this binding.
	// Unset before the first EndpointRelease is cut; the controller leaves the
	// binding pending until set.
	// +optional
	EndpointRelease string `json:"endpointRelease,omitempty"`

	// EndpointTypeEnvironmentConfigs provides per-environment values for the schema
	// declared on the referenced EndpointType (or ClusterEndpointType). Validated
	// against EndpointType.spec.environmentConfigs by the binding controller;
	// failures surface via status.conditions.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	EndpointTypeEnvironmentConfigs *runtime.RawExtension `json:"endpointTypeEnvironmentConfigs,omitempty"`
}

// EndpointReleaseBindingOwner identifies the endpoint this EndpointReleaseBinding belongs to.
// Mirrors EndpointReleaseOwner for namespace-disambiguation: a namespace can host
// multiple projects, so a bare endpoint name is ambiguous.
type EndpointReleaseBindingOwner struct {
	// ProjectName is the name of the project that owns this endpoint.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// EndpointName is the name of the Endpoint.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	EndpointName string `json:"endpointName"`
}

// EndpointReleaseBindingStatus defines the observed state of EndpointReleaseBinding.
type EndpointReleaseBindingStatus struct {
	// Conditions represent the latest available observations of the binding's state.
	// Includes BackendsResolved, Synced, and Ready (aggregate). observedGeneration
	// is set per-condition (project convention).
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Backends holds the resolution result of each backendRef in this binding's
	// environment. A backend that cannot be resolved is reported with
	// resolved=false and a reason, rather than silently receiving its share of
	// traffic.
	// +optional
	// +listType=map
	// +listMapKey=name
	Backends []ResolvedEndpointBackend `json:"backends,omitempty"`
}

// ResolvedEndpointBackend is the resolution of a single EndpointBackendRef in an
// environment, populated by the binding controller.
type ResolvedEndpointBackend struct {
	// Name matches the name of the EndpointBackendRef this entry resolves.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Kind is the kind of the resolved backend.
	// +kubebuilder:validation:Required
	Kind EndpointBackendKind `json:"kind"`

	// Weight is the effective weight applied to this backend.
	// +optional
	Weight int32 `json:"weight,omitempty"`

	// Resolved indicates whether the backend was found and can receive traffic
	// in this environment.
	Resolved bool `json:"resolved"`

	// ServiceURL is the in-cluster address of the backend in this environment.
	// Set only when resolved is true.
	// +optional
	ServiceURL *EndpointURL `json:"serviceURL,omitempty"`

	// Reason is a CamelCase code explaining why the backend could not be
	// resolved. Set only when resolved is false.
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message is a human-readable explanation accompanying Reason.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=eprb;eprbs
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.spec.owner.endpointName`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="Release",type=string,JSONPath=`.spec.endpointRelease`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EndpointReleaseBinding is the Schema for the endpointreleasebindings API.
// Pins an EndpointRelease to an Environment, resolves its backends to the
// environment's component services, and renders the EndpointType's routing
// resources. Authored externally; not managed by the Endpoint controller.
type EndpointReleaseBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointReleaseBindingSpec   `json:"spec,omitempty"`
	Status EndpointReleaseBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EndpointReleaseBindingList contains a list of EndpointReleaseBinding.
type EndpointReleaseBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EndpointReleaseBinding `json:"items"`
}

// GetConditions returns the conditions from the status.
func (e *EndpointReleaseBinding) GetConditions() []metav1.Condition {
	return e.Status.Conditions
}

// SetConditions sets the conditions in the status.
func (e *EndpointReleaseBinding) SetConditions(conditions []metav1.Condition) {
	e.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&EndpointReleaseBinding{}, &EndpointReleaseBindingList{})
}
