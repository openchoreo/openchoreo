// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BuildPlaneSpec defines the desired state of BuildPlane.
type BuildPlaneSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Owner BuildPlaneOwner `json:"owner"`

	// KubernetesCluster defines the Kubernetes cluster where build workloads (e.g., Argo Workflows) will be executed.
	KubernetesCluster KubernetesClusterSpec `json:"kubernetesCluster"`
}

type BuildPlaneOwner struct {
	// +kubebuilder:validation:MinLength=1
	OrganizationName string `json:"organizationName"`
}

// BuildPlaneStatus defines the observed state of BuildPlane.
type BuildPlaneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// BuildPlane is the Schema for the buildplanes API.
type BuildPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuildPlaneSpec   `json:"spec,omitempty"`
	Status BuildPlaneStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BuildPlaneList contains a list of BuildPlane.
type BuildPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BuildPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BuildPlane{}, &BuildPlaneList{})
}
