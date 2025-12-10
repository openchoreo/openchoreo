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

	// ClusterAgent specifies the configuration for cluster agent-based communication with the downstream cluster
	// The control plane communicates with the downstream cluster through a WebSocket cluster agent via ClusterGateway
	// This is a required field as cluster agent mode is the only supported communication method
	ClusterAgent ClusterAgentConfig `json:"clusterAgent"`

	// ObservabilityPlaneRef specifies the name of the ObservabilityPlane for this BuildPlane.
	// +optional
	ObservabilityPlaneRef string `json:"observabilityPlaneRef,omitempty"`
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
