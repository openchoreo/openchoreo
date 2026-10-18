// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EndpointReleaseSpec defines the desired state of EndpointRelease.
// An EndpointRelease is an immutable snapshot of Endpoint.spec and the
// referenced EndpointType (or ClusterEndpointType) spec at the time it was cut.
// Created exclusively by the Endpoint controller.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="EndpointRelease spec is immutable"
type EndpointReleaseSpec struct {
	// Owner identifies the endpoint and project this EndpointRelease belongs to.
	// +kubebuilder:validation:Required
	Owner EndpointReleaseOwner `json:"owner"`

	// EndpointType is a frozen snapshot of the EndpointType or ClusterEndpointType
	// resource at the time of the release. Records the kind and name of the
	// original resource alongside its full spec so consumers can distinguish
	// namespace-scoped EndpointTypes from cluster-scoped ClusterEndpointTypes.
	// +kubebuilder:validation:Required
	EndpointType EndpointReleaseEndpointType `json:"endpointType"`

	// Visibility holds the snapshot of the visibilities requested by the
	// Endpoint at release time.
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	Visibility []EndpointVisibility `json:"visibility"`

	// BackendRefs holds the snapshot of backend references and weights from the
	// Endpoint spec at release time.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +listType=map
	// +listMapKey=name
	BackendRefs []EndpointBackendRef `json:"backendRefs"`

	// Schema holds the snapshot of the explicit schema override from the
	// Endpoint spec at release time. Unset when the Endpoint resolves its schema
	// from the backend.
	// +optional
	Schema *Schema `json:"schema,omitempty"`

	// Parameters holds the snapshot of parameter values from the Endpoint spec at
	// release time. The schema for these values is defined by the EndpointType's
	// parameters schema.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// EndpointReleaseEndpointType is the frozen snapshot of an EndpointType or
// ClusterEndpointType resource stored on an EndpointRelease. Preserves Kind and
// Name of the original resource alongside its full spec so an EndpointType and a
// ClusterEndpointType with the same name can coexist.
type EndpointReleaseEndpointType struct {
	// Kind identifies whether this is a namespace-scoped EndpointType or a
	// cluster-scoped ClusterEndpointType.
	// +kubebuilder:validation:Required
	Kind EndpointTypeRefKind `json:"kind"`

	// Name is the name of the EndpointType or ClusterEndpointType resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Spec is the frozen specification of the (Cluster)EndpointType at the time
	// of this EndpointRelease. ClusterEndpointTypeSpec currently mirrors
	// EndpointTypeSpec so both kinds are stored under the namespaced type. If
	// ClusterEndpointTypeSpec gains cluster-only fields, snapshots from a
	// ClusterEndpointType source will lose them; revisit this field then. Mirrors
	// the ResourceReleaseResourceType precedent.
	// +kubebuilder:validation:Required
	Spec EndpointTypeSpec `json:"spec"`
}

// EndpointReleaseOwner identifies the endpoint this EndpointRelease belongs to.
type EndpointReleaseOwner struct {
	// ProjectName is the name of the project that owns this endpoint.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// EndpointName is the name of the Endpoint.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	EndpointName string `json:"endpointName"`
}

// EndpointReleaseStatus defines the observed state of EndpointRelease.
type EndpointReleaseStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=epr;eprs
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.spec.owner.endpointName`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EndpointRelease is the Schema for the endpointreleases API. An immutable
// snapshot of an Endpoint and its referenced (Cluster)EndpointType at a point
// in time. Created by the Endpoint controller when the hash of Endpoint.spec +
// (Cluster)EndpointType.spec changes.
type EndpointRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointReleaseSpec   `json:"spec,omitempty"`
	Status EndpointReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EndpointReleaseList contains a list of EndpointRelease.
type EndpointReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EndpointRelease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EndpointRelease{}, &EndpointReleaseList{})
}
