// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterBuildPlaneSpec defines the desired state of ClusterBuildPlane.
// This is a cluster-scoped version of BuildPlaneSpec, allowing platform admins
// to define build planes that can be referenced across namespaces.
type ClusterBuildPlaneSpec struct {
	// PlaneID identifies the logical plane this CR connects to.
	// Multiple ClusterBuildPlane CRs can share the same planeID to connect to the same physical cluster
	// while maintaining separate configurations for multi-tenancy scenarios.
	// This field is required and must uniquely identify the physical cluster agent connection.
	// Format: lowercase alphanumeric characters, hyphens allowed, max 63 characters.
	// Examples: "shared-builder", "ci-cluster", "us-west-2"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	PlaneID string `json:"planeID"`

	// ClusterAgent specifies the configuration for cluster agent-based communication
	// The cluster agent establishes a WebSocket connection to the control plane's cluster gateway
	// This field is mandatory - all build planes must use cluster agent communication
	ClusterAgent ClusterAgentConfig `json:"clusterAgent"`

	// SecretStoreRef specifies the ESO ClusterSecretStore to use in the build plane
	// +optional
	SecretStoreRef *SecretStoreRef `json:"secretStoreRef,omitempty"`

	// ObservabilityPlaneRef specifies the ClusterObservabilityPlane for this ClusterBuildPlane.
	// Since this is a cluster-scoped resource, it can only reference cluster-scoped ClusterObservabilityPlane.
	// Namespace-scoped ObservabilityPlane references are NOT supported for cluster-scoped resources.
	//
	// Default behavior:
	// - If not specified, the controller defaults to a ClusterObservabilityPlane named "default"
	// - If the "default" ClusterObservabilityPlane does not exist, reconciliation fails with an error
	//
	// The kind field must be "ClusterObservabilityPlane".
	// +optional
	// +kubebuilder:validation:XValidation:rule="!has(self.kind) || self.kind == 'ClusterObservabilityPlane'",message="ClusterBuildPlane can only reference ClusterObservabilityPlane"
	ObservabilityPlaneRef *ClusterObservabilityPlaneRef `json:"observabilityPlaneRef,omitempty"`
}

// ClusterBuildPlaneStatus defines the observed state of ClusterBuildPlane.
type ClusterBuildPlaneStatus struct {
	// ObservedGeneration reflects the generation of the most recently observed ClusterBuildPlane.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the current state of the ClusterBuildPlane resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// AgentConnection tracks the status of cluster agent connections to this build plane
	// +optional
	AgentConnection *AgentConnectionStatus `json:"agentConnection,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cbp;cbps

// ClusterBuildPlane is the Schema for the clusterbuildplanes API.
// It is a cluster-scoped version of BuildPlane, allowing platform administrators
// to define build plane configurations that can be referenced across multiple namespaces.
type ClusterBuildPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterBuildPlaneSpec   `json:"spec,omitempty"`
	Status ClusterBuildPlaneStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterBuildPlaneList contains a list of ClusterBuildPlane.
type ClusterBuildPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterBuildPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterBuildPlane{}, &ClusterBuildPlaneList{})
}

// GetConditions returns the conditions of the ClusterBuildPlane.
func (c *ClusterBuildPlane) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

// SetConditions sets the conditions of the ClusterBuildPlane.
func (c *ClusterBuildPlane) SetConditions(conditions []metav1.Condition) {
	c.Status.Conditions = conditions
}
