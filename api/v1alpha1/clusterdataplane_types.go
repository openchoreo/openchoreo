// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterDataPlaneSpec defines the desired state of ClusterDataPlane.
// This is a cluster-scoped version of DataPlaneSpec, allowing platform admins
// to define data planes that can be referenced across namespaces.
type ClusterDataPlaneSpec struct {
	// PlaneID identifies the logical plane this CR connects to.
	// Multiple ClusterDataPlane CRs can share the same planeID to connect to the same physical cluster
	// while maintaining separate configurations for multi-tenancy scenarios.
	// This field is required and must uniquely identify the physical cluster agent connection.
	// Format: lowercase alphanumeric characters, hyphens allowed, max 63 characters.
	// Examples: "prod-cluster", "shared-dataplane", "us-east-1"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	PlaneID string `json:"planeID"`

	// ClusterAgent specifies the configuration for cluster agent-based communication
	// The cluster agent establishes a WebSocket connection to the control plane's cluster gateway
	// This field is mandatory - all data planes must use cluster agent communication
	ClusterAgent ClusterAgentConfig `json:"clusterAgent"`

	// Gateway specifies the configuration for the API gateway in this DataPlane.
	Gateway GatewaySpec `json:"gateway"`

	// ImagePullSecretRefs contains references to SecretReference resources
	// These will be converted to ExternalSecrets and added as imagePullSecrets to all deployments
	// +optional
	ImagePullSecretRefs []string `json:"imagePullSecretRefs,omitempty"`

	// SecretStoreRef specifies the ESO ClusterSecretStore to use in the data plane
	// +optional
	SecretStoreRef *SecretStoreRef `json:"secretStoreRef,omitempty"`

	// ObservabilityPlaneRef specifies the ClusterObservabilityPlane for this ClusterDataPlane.
	// Since this is a cluster-scoped resource, it can only reference cluster-scoped ClusterObservabilityPlane.
	// Namespace-scoped ObservabilityPlane references are NOT supported for cluster-scoped resources.
	//
	// Default behavior:
	// - If not specified, the system looks for a ClusterObservabilityPlane named "default"
	// - If "default" ClusterObservabilityPlane doesn't exist, observability features are disabled
	//
	// The kind field must be "ClusterObservabilityPlane".
	// +optional
	// +kubebuilder:validation:XValidation:rule="!has(self.kind) || self.kind == 'ClusterObservabilityPlane'",message="ClusterDataPlane can only reference ClusterObservabilityPlane"
	ObservabilityPlaneRef *ClusterObservabilityPlaneRef `json:"observabilityPlaneRef,omitempty"`
}

// ClusterDataPlaneStatus defines the observed state of ClusterDataPlane.
type ClusterDataPlaneStatus struct {
	// ObservedGeneration reflects the generation of the most recently observed ClusterDataPlane.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the current state of the ClusterDataPlane resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// AgentConnection tracks the status of cluster agent connections to this data plane
	// +optional
	AgentConnection *AgentConnectionStatus `json:"agentConnection,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cdp;cdps

// ClusterDataPlane is the Schema for the clusterdataplanes API.
// It is a cluster-scoped version of DataPlane, allowing platform administrators
// to define data plane configurations that can be referenced across multiple namespaces.
type ClusterDataPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterDataPlaneSpec   `json:"spec,omitempty"`
	Status ClusterDataPlaneStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterDataPlaneList contains a list of ClusterDataPlane.
type ClusterDataPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterDataPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterDataPlane{}, &ClusterDataPlaneList{})
}

// GetConditions returns the conditions of the ClusterDataPlane.
func (c *ClusterDataPlane) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

// SetConditions sets the conditions of the ClusterDataPlane.
func (c *ClusterDataPlane) SetConditions(conditions []metav1.Condition) {
	c.Status.Conditions = conditions
}
