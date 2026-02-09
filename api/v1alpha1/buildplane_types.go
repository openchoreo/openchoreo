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

	// PlaneID identifies the logical plane this CR connects to.
	// Multiple BuildPlane CRs can share the same planeID to connect to the same physical cluster
	// while maintaining separate configurations for multi-tenancy scenarios.
	// If not specified, defaults to the CR name for backwards compatibility.
	// Format: lowercase alphanumeric characters, hyphens allowed, max 63 characters.
	// Examples: "shared-builder", "ci-cluster", "us-west-2"
	// +optional
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	PlaneID string `json:"planeID,omitempty"`

	// ClusterAgent specifies the configuration for cluster agent-based communication
	// The cluster agent establishes a WebSocket connection to the control plane's cluster gateway
	// This field is mandatory - all build planes must use cluster agent communication
	ClusterAgent ClusterAgentConfig `json:"clusterAgent"`

	// SecretStoreRef specifies the ESO ClusterSecretStore to use in the data plane
	// +optional
	SecretStoreRef *SecretStoreRef `json:"secretStoreRef,omitempty"`

	// ObservabilityPlaneRef specifies the ObservabilityPlane or ClusterObservabilityPlane for this BuildPlane.
	// If not specified, defaults to an ObservabilityPlane named "default" in the same namespace.
	// +optional
	ObservabilityPlaneRef *ObservabilityPlaneRef `json:"observabilityPlaneRef,omitempty"`
}

// BuildPlaneStatus defines the observed state of BuildPlane.
type BuildPlaneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ObservedGeneration is the generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the BuildPlane's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// AgentConnection tracks the status of cluster agent connections to this build plane
	// +optional
	AgentConnection *AgentConnectionStatus `json:"agentConnection,omitempty"`
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
