// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EndpointSpec defines the desired state of Endpoint.
type EndpointSpec struct {
	// Owner identifies the Project this Endpoint belongs to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.owner is immutable"
	Owner EndpointOwner `json:"owner"`

	// Type references the EndpointType or ClusterEndpointType template for this Endpoint.
	// Kind defaults to EndpointType (namespaced); ClusterEndpointType is also allowed.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.type cannot be changed after creation"
	Type EndpointTypeRef `json:"type"`

	// Visibility lists the scopes this Endpoint exposes its backends to. It is
	// declared rather than inferred from rendered routes, so an EndpointType can
	// render one route per requested visibility and the set of route slots this
	// Endpoint occupies is known before anything is rendered. The project scope
	// is implicit for every workload endpoint and is not a route an Endpoint
	// renders, so it is not accepted here.
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	// +kubebuilder:validation:XValidation:rule="self.all(v, v != 'project')",message="project visibility is implicit for every workload endpoint and cannot be requested on an Endpoint"
	Visibility []EndpointVisibility `json:"visibility"`

	// BackendRefs lists the backends this Endpoint routes traffic to, with
	// relative weights. Like a Service selecting Pods, the Endpoint does not own
	// the referenced components.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +listType=map
	// +listMapKey=name
	BackendRefs []EndpointBackendRef `json:"backendRefs"`

	// Schema is the API definition this Endpoint exposes. When unset it resolves
	// from the backend's Workload endpoint schema at render time rather than
	// being copied here, so a new build cannot leave the Endpoint holding a stale
	// contract. Set it explicitly when backends disagree or the exposed surface
	// differs from the backend's.
	// +optional
	Schema *Schema `json:"schema,omitempty"`

	// Parameters contains values for the parameter schema declared on the referenced
	// EndpointType (or ClusterEndpointType). Validated against the schema by the
	// Endpoint controller; failures surface via status.conditions.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// EndpointOwner identifies the project that owns an Endpoint.
type EndpointOwner struct {
	// ProjectName is the name of the Project this Endpoint belongs to.
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
}

// EndpointBackendKind identifies what an EndpointBackendRef points at.
// +kubebuilder:validation:Enum=Component
type EndpointBackendKind string

const (
	// EndpointBackendKindComponent references a workload endpoint of a Component.
	EndpointBackendKindComponent EndpointBackendKind = "Component"
)

// EndpointBackendRef is one traffic backend of an Endpoint. It is a
// discriminated union on kind; exactly the payload matching kind must be set.
// +kubebuilder:validation:XValidation:rule="self.kind != 'Component' || has(self.component)",message="component must be set when kind is Component"
type EndpointBackendRef struct {
	// Name identifies this backend within the Endpoint. It is the list's map key,
	// the handle status reports resolution against, and what a template uses to
	// single out one backend.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// Kind is the kind of backend this entry references.
	// +optional
	// +kubebuilder:default=Component
	Kind EndpointBackendKind `json:"kind,omitempty"`

	// Component references a workload endpoint of a Component in the Endpoint's
	// project. Required when kind is Component.
	// +optional
	Component *EndpointComponentBackend `json:"component,omitempty"`

	// Weight is the proportion of traffic sent to this backend relative to the
	// sum of all backend weights. Follows Gateway API backendRef weight semantics:
	// 0 sends no traffic to this backend.
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000000
	Weight *int32 `json:"weight,omitempty"`
}

// EndpointComponentBackend references a named workload endpoint of a Component.
type EndpointComponentBackend struct {
	// Name is the name of the Component that serves this backend, in the
	// Endpoint's project.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// Endpoint is the name of the endpoint in the component's
	// Workload.spec.endpoints to route to. Determines the backend port and protocol.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`
}

// EndpointStatus defines the observed state of Endpoint.
type EndpointStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the Endpoint's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LatestRelease is the most recent EndpointRelease for this Endpoint.
	// +optional
	LatestRelease *LatestEndpointRelease `json:"latestRelease,omitempty"`
}

// LatestEndpointRelease identifies the most recent EndpointRelease for an Endpoint.
type LatestEndpointRelease struct {
	// Name of the EndpointRelease resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Hash is the content hash of Endpoint.spec + EndpointType/ClusterEndpointType.spec
	// captured at release time.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Hash string `json:"hash"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type.name`
// +kubebuilder:printcolumn:name="Release",type=string,JSONPath=`.status.latestRelease.name`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Endpoint is the Schema for the endpoints API.
// An Endpoint routes traffic across one or more backends using weights,
// rendered through an EndpointType or ClusterEndpointType. It is concerned with
// traffic routing only; the compute side (ports, network policies) stays on
// Workload.spec.endpoints.
// The plural "endpoints" and the short name "ep" are taken by core v1 Endpoints
// in kubectl, so this kind has no short name; use "endpoints.openchoreo.dev".
type Endpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointSpec   `json:"spec,omitempty"`
	Status EndpointStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EndpointList contains a list of Endpoint.
type EndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Endpoint `json:"items"`
}

func (e *Endpoint) GetConditions() []metav1.Condition {
	return e.Status.Conditions
}

func (e *Endpoint) SetConditions(conditions []metav1.Condition) {
	e.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&Endpoint{}, &EndpointList{})
}
