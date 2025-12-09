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

// KubernetesClusterSpec defines the configuration for the target Kubernetes cluster
type KubernetesClusterSpec struct {
	// Server is the URL of the Kubernetes API server
	Server string `json:"server"`
	// TLS contains the TLS configuration for the connection
	TLS KubernetesTLS `json:"tls"`
	// Auth contains the authentication configuration
	Auth KubernetesAuth `json:"auth"`
}

// KubernetesTLS defines the TLS configuration for the Kubernetes connection
type KubernetesTLS struct {
	// CA contains the CA certificate configuration
	CA ValueFrom `json:"ca"`
}

// KubernetesAuth defines the authentication configuration for the Kubernetes cluster
type KubernetesAuth struct {
	// MTLS contains the certificate-based authentication configuration
	// +optional
	MTLS *MTLSAuth `json:"mtls,omitempty"`
	// BearerToken contains the bearer token authentication configuration
	// +optional
	BearerToken *ValueFrom `json:"bearerToken,omitempty"`
}

// MTLSAuth defines certificate-based authentication (mTLS)
type MTLSAuth struct {
	// ClientCert contains the client certificate configuration
	ClientCert ValueFrom `json:"clientCert"`
	// ClientKey contains the client private key configuration
	ClientKey ValueFrom `json:"clientKey"`
}

// GatewaySpec defines the gateway configuration for the data plane
type GatewaySpec struct {
	// Public virtual host for the gateway
	PublicVirtualHost string `json:"publicVirtualHost"`
	// Organization-specific virtual host for the gateway
	OrganizationVirtualHost string `json:"organizationVirtualHost"`
}

// SecretStoreRef defines a reference to an External Secrets Operator ClusterSecretStore
type SecretStoreRef struct {
	// Name of the ClusterSecretStore resource in the data plane cluster
	Name string `json:"name"`
}

// AgentConfig defines the configuration for agent-based communication
// This configuration is specified in DataPlane or BuildPlane CRs on the control plane
type AgentConfig struct {
	// Enabled indicates whether agent-based communication is enabled
	// If false, direct Kubernetes API access is used via KubernetesCluster config
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// ClientCA contains the CA certificate used to verify the agent's client certificate
	// This allows per-plane CA configuration for enhanced security
	// The CA certificate should be base64-encoded PEM format
	// +optional
	ClientCA *ValueFrom `json:"clientCA,omitempty"`
}

// DataPlaneSpec defines the desired state of a DataPlane.
type DataPlaneSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ImagePullSecretRefs contains references to SecretReference resources
	// These will be converted to ExternalSecrets and added as imagePullSecrets to all deployments
	// +optional
	ImagePullSecretRefs []string `json:"imagePullSecretRefs,omitempty"`

	// SecretStoreRef specifies the ESO ClusterSecretStore to use in the data plane
	// +optional
	SecretStoreRef *SecretStoreRef `json:"secretStoreRef,omitempty"`

	// Agent specifies the configuration for agent-based communication with the downstream cluster
	// When enabled, the control plane communicates with the downstream cluster through a WebSocket agent
	// instead of direct Kubernetes API access
	// +optional
	Agent *AgentConfig `json:"agent,omitempty"`

	// KubernetesCluster defines the target Kubernetes cluster where workloads should be deployed.
	// This field is optional when Agent.Enabled is true
	// +optional
	KubernetesCluster *KubernetesClusterSpec `json:"kubernetesCluster,omitempty"`
	// Gateway specifies the configuration for the API gateway in this DataPlane.
	Gateway GatewaySpec `json:"gateway"`

	// ObservabilityPlaneRef specifies the name of the ObservabilityPlane for this DataPlane.
	// +optional
	ObservabilityPlaneRef string `json:"observabilityPlaneRef,omitempty"`
}

// DataPlaneStatus defines the observed state of DataPlane.
type DataPlaneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
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
