// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterObservabilityPlaneSpec defines the desired state of ClusterObservabilityPlane.
// This is a cluster-scoped version of ObservabilityPlaneSpec, allowing platform admins
// to define observability planes that can be referenced across namespaces.
type ClusterObservabilityPlaneSpec struct {
	// PlaneID identifies the logical plane this CR connects to.
	// Multiple ClusterObservabilityPlane CRs can share the same planeID to connect to the same physical cluster
	// while maintaining separate configurations for multi-tenancy scenarios.
	// This field is required and must uniquely identify the physical cluster agent connection.
	// Format: lowercase alphanumeric characters, hyphens allowed, max 63 characters.
	// Examples: "shared-obs", "monitoring-cluster", "eu-central-1"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	PlaneID string `json:"planeID"`

	// ClusterAgent specifies the configuration for cluster agent-based communication
	// The cluster agent establishes a WebSocket connection to the control plane's cluster gateway
	// This field is mandatory - all observability planes must use cluster agent communication
	ClusterAgent ClusterAgentConfig `json:"clusterAgent"`

	// ObserverURL is the base URL of the Observer API in the observability plane cluster
	// +required
	ObserverURL string `json:"observerURL"`

	// RCAAgentURL is the base URL of the RCA Agent API in the observability plane cluster
	// +optional
	RCAAgentURL string `json:"rcaAgentURL,omitempty"`
}

// ClusterObservabilityPlaneStatus defines the observed state of ClusterObservabilityPlane.
type ClusterObservabilityPlaneStatus struct {
	// ObservedGeneration reflects the generation of the most recently observed ClusterObservabilityPlane.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the current state of the ClusterObservabilityPlane resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// AgentConnection tracks the status of cluster agent connections to this observability plane
	// +optional
	AgentConnection *AgentConnectionStatus `json:"agentConnection,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cop;cops

// ClusterObservabilityPlane is the Schema for the clusterobservabilityplanes API.
// It is a cluster-scoped version of ObservabilityPlane, allowing platform administrators
// to define observability plane configurations that can be referenced across multiple namespaces.
type ClusterObservabilityPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterObservabilityPlaneSpec   `json:"spec,omitempty"`
	Status ClusterObservabilityPlaneStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterObservabilityPlaneList contains a list of ClusterObservabilityPlane.
type ClusterObservabilityPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterObservabilityPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterObservabilityPlane{}, &ClusterObservabilityPlaneList{})
}

// GetConditions returns the conditions of the ClusterObservabilityPlane.
func (c *ClusterObservabilityPlane) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

// SetConditions sets the conditions of the ClusterObservabilityPlane.
func (c *ClusterObservabilityPlane) SetConditions(conditions []metav1.Condition) {
	c.Status.Conditions = conditions
}
