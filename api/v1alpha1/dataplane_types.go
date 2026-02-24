// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretKeyReference defines a reference to a specific key in a Kubernetes secret
type SecretKeyReference struct {
	// Name of the secret
	Name string `json:"name"`
	// Namespace of the secret (optional, defaults to same namespace as parent resource)
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// Key is the key within the secret
	Key string `json:"key"`
}

// ValueFrom defines a common pattern for referencing secrets or providing inline values
type ValueFrom struct {
	// SecretRef is a reference to a secret containing the value
	// +optional
	SecretRef *SecretKeyReference `json:"secretRef,omitempty"`
	// Value is the inline value (optional fallback)
	// +optional
	Value string `json:"value,omitempty"`
}

// ClusterAgentConfig defines the configuration for cluster agent-based communication
// The cluster agent establishes a WebSocket connection to the control plane's cluster gateway
type ClusterAgentConfig struct {
	// ClientCA contains the CA certificate used to verify the agent's client certificate
	// This allows per-plane CA configuration for enhanced security
	ClientCA ValueFrom `json:"clientCA"`
}

// GatewayListenerSpec defines the configuration for a single gateway listener.
type GatewayListenerSpec struct {
	// ListenerName is the name of the listener on the Gateway resource.
	// +optional
	ListenerName string `json:"listenerName,omitempty"`
	// Port is the port number for this listener.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
	// Host is the virtual host for this listener.
	Host string `json:"host"`
}

// GatewayEndpointSpec defines the configuration for a gateway endpoint (name, namespace, and listeners).
type GatewayEndpointSpec struct {
	// Name is the name of the Gateway resource.
	// +optional
	Name string `json:"name,omitempty"`
	// Namespace is the namespace of the Gateway resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// HTTP defines the HTTP listener configuration.
	// +optional
	HTTP *GatewayListenerSpec `json:"http,omitempty"`
	// HTTPS defines the HTTPS listener configuration.
	// +optional
	HTTPS *GatewayListenerSpec `json:"https,omitempty"`
	// TLS defines the TLS listener configuration.
	// +optional
	TLS *GatewayListenerSpec `json:"tls,omitempty"`
}

// GatewayNetworkSpec defines external and internal gateway endpoint configurations.
type GatewayNetworkSpec struct {
	// External defines the externally accessible gateway endpoint.
	// +optional
	External *GatewayEndpointSpec `json:"external,omitempty"`
	// Internal defines the internally accessible gateway endpoint.
	// +optional
	Internal *GatewayEndpointSpec `json:"internal,omitempty"`
}

// GatewaySpec defines the gateway configuration for the data plane.
type GatewaySpec struct {
	// Ingress defines the ingress gateway configuration.
	// +optional
	Ingress *GatewayNetworkSpec `json:"ingress,omitempty"`
	// Egress defines the egress gateway configuration.
	// +optional
	Egress *GatewayNetworkSpec `json:"egress,omitempty"`
}

// SecretStoreRef defines a reference to an External Secrets Operator ClusterSecretStore
type SecretStoreRef struct {
	// Name of the ClusterSecretStore resource in the data plane cluster
	Name string `json:"name"`
}

// DataPlaneSpec defines the desired state of a DataPlane.
type DataPlaneSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// PlaneID identifies the logical plane this CR connects to.
	// Multiple DataPlane CRs can share the same planeID to connect to the same physical cluster
	// while maintaining separate configurations for multi-tenancy scenarios.
	// If not specified, defaults to the CR name for backwards compatibility.
	// Format: lowercase alphanumeric characters, hyphens allowed, max 63 characters.
	// Examples: "prod-cluster", "shared-dataplane", "us-east-1"
	// +optional
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	PlaneID string `json:"planeID,omitempty"`

	// ClusterAgent specifies the configuration for cluster agent-based communication
	// The cluster agent establishes a WebSocket connection to the control plane's cluster gateway
	// This field is mandatory - all data planes must use cluster agent communication
	ClusterAgent ClusterAgentConfig `json:"clusterAgent"`

	// Gateway specifies the configuration for the API gateway in this DataPlane.
	// +optional
	Gateway GatewaySpec `json:"gateway,omitempty"`

	// ImagePullSecretRefs contains references to SecretReference resources
	// These will be converted to ExternalSecrets and added as imagePullSecrets to all deployments
	// +optional
	ImagePullSecretRefs []string `json:"imagePullSecretRefs,omitempty"`

	// SecretStoreRef specifies the ESO ClusterSecretStore to use in the data plane
	// +optional
	SecretStoreRef *SecretStoreRef `json:"secretStoreRef,omitempty"`

	// ObservabilityPlaneRef specifies the ObservabilityPlane or ClusterObservabilityPlane for this DataPlane.
	// If not specified, defaults to an ObservabilityPlane named "default" in the same namespace.
	// +optional
	ObservabilityPlaneRef *ObservabilityPlaneRef `json:"observabilityPlaneRef,omitempty"`
}

// AgentConnectionStatus tracks the status of cluster agent connections
type AgentConnectionStatus struct {
	// Connected indicates whether any cluster agent is currently connected
	Connected bool `json:"connected"`

	// ConnectedAgents is the number of cluster agents currently connected
	ConnectedAgents int `json:"connectedAgents"`

	// LastConnectedTime is when an agent last successfully connected
	// +optional
	LastConnectedTime *metav1.Time `json:"lastConnectedTime,omitempty"`

	// LastDisconnectedTime is when the last agent disconnected
	// +optional
	LastDisconnectedTime *metav1.Time `json:"lastDisconnectedTime,omitempty"`

	// LastHeartbeatTime is when the control plane last received any communication from an agent
	// +optional
	LastHeartbeatTime *metav1.Time `json:"lastHeartbeatTime,omitempty"`

	// Message provides additional information about the agent connection status
	// +optional
	Message string `json:"message,omitempty"`
}

// DataPlaneStatus defines the observed state of DataPlane.
type DataPlaneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`

	// AgentConnection tracks the status of cluster agent connections to this data plane
	// +optional
	AgentConnection *AgentConnectionStatus `json:"agentConnection,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=dp;dps

// DataPlane is the Schema for the dataplanes API.
type DataPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataPlaneSpec   `json:"spec,omitempty"`
	Status DataPlaneStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DataPlaneList contains a list of DataPlane.
type DataPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataPlane{}, &DataPlaneList{})
}

func (d *DataPlane) GetConditions() []metav1.Condition {
	return d.Status.Conditions
}

func (d *DataPlane) SetConditions(conditions []metav1.Condition) {
	d.Status.Conditions = conditions
}
