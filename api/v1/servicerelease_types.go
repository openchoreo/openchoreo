// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServiceReleaseSpec defines the desired state of ServiceRelease.
type ServiceReleaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ServiceRelease. Edit servicerelease_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// ServiceReleaseStatus defines the observed state of ServiceRelease.
type ServiceReleaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ServiceRelease is the Schema for the servicereleases API.
type ServiceRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceReleaseSpec   `json:"spec,omitempty"`
	Status ServiceReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceReleaseList contains a list of ServiceRelease.
type ServiceReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceRelease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceRelease{}, &ServiceReleaseList{})
}
