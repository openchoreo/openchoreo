// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WebApplicationReleaseSpec defines the desired state of WebApplicationRelease.
type WebApplicationReleaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Owner WebApplicationOwner `json:"owner"`
	// +kubebuilder:validation:MinLength=1
	EnvironmentName string `json:"environmentName"`

	// Scalable resource template approach (KRO-inspired)
	// Supports any Kubernetes resource type including HPA, PDB, NetworkPolicy, CRDs, etc. that can
	// be applied to the data plane.
	// +kubebuilder:validation:Optional
	Resources []Resource `json:"resources,omitempty"`
}

// WebApplicationReleaseStatus defines the observed state of WebApplicationRelease.
type WebApplicationReleaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// WebApplicationRelease is the Schema for the webapplicationreleases API.
type WebApplicationRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebApplicationReleaseSpec   `json:"spec,omitempty"`
	Status WebApplicationReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WebApplicationReleaseList contains a list of WebApplicationRelease.
type WebApplicationReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebApplicationRelease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WebApplicationRelease{}, &WebApplicationReleaseList{})
}
