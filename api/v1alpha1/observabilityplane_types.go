// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ObservabilityPlaneSpec defines the desired state of ObservabilityPlane.
type ObservabilityPlaneSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Agent specifies the configuration for agent-based communication with the downstream cluster
	// When enabled, the control plane communicates with the downstream cluster through a WebSocket agent
	// instead of direct Kubernetes API access
	// +optional
	Agent *AgentConfig `json:"agent,omitempty"`

	// ObserverURL is the base URL of the Observer API in the observability plane cluster
	// +required
	ObserverURL string `json:"observerURL"`
}

// ObservabilityPlaneStatus defines the observed state of ObservabilityPlane.
type ObservabilityPlaneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ObservabilityPlane is the Schema for the observabilityplanes API.
type ObservabilityPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ObservabilityPlaneSpec   `json:"spec,omitempty"`
	Status ObservabilityPlaneStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ObservabilityPlaneList contains a list of ObservabilityPlane.
type ObservabilityPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ObservabilityPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ObservabilityPlane{}, &ObservabilityPlaneList{})
}
