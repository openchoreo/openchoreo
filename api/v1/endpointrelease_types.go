// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type EndpointReleaseOwner struct {
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// EndpointReleaseSpec defines the desired state of EndpointRelease.
type EndpointReleaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Owner EndpointReleaseOwner `json:"owner"`
	// +kubebuilder:validation:MinLength=1
	EnvironmentName string `json:"environmentName"`

	Type EndpointType `json:"type"`

	// Scalable resource template approach (KRO-inspired)
	// Supports any Kubernetes resource type including HTTPRoute, Gateway, Ingress, Service, NetworkPolicy, etc. that can
	// be applied to the data plane for endpoint configuration.
	// +kubebuilder:validation:Optional
	Resources []Resource `json:"resources,omitempty"`
}

// EndpointReleaseStatus defines the observed state of EndpointRelease.
type EndpointReleaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EndpointRelease is the Schema for the endpointreleases API.
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
