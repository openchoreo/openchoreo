// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KubernetesClusterSpec defines the configuration for the target Kubernetes cluster
type KubernetesClusterSpec struct {
	// Name of the Kubernetes cluster
	Name string `json:"name"`
	// Credentials contains the authentication details for accessing the Kubernetes API server.
	Credentials APIServerCredentials `json:"credentials"`
}

// APIServerCredentials holds the TLS credentials to connect securely with a Kubernetes API server.
type APIServerCredentials struct {
	// APIServerURL is the URL of the Kubernetes API server.
	APIServerURL string `json:"apiServerURL"`
	// CACert is the base64-encoded CA certificate used to verify the server's certificate.
	CACert string `json:"caCert"`
	// ClientCert is the base64-encoded client certificate used for authentication.
	ClientCert string `json:"clientCert"`
	// ClientKey is the base64-encoded private key corresponding to the client certificate.
	ClientKey string `json:"clientKey"`
}

// GatewaySpec defines the gateway configuration for the data plane
type GatewaySpec struct {
	// Public virtual host for the gateway
	PublicVirtualHost string `json:"publicVirtualHost"`
	// Organization-specific virtual host for the gateway
	OrganizationVirtualHost string `json:"organizationVirtualHost"`
}

// Registry contains the list of secrets used to authenticate against container registries.
type Registry struct {
	// Unauthenticated lists registry domains that do not require authentication (e.g., public registries).
	Unauthenticated []string `json:"unauthenticated,omitempty"`
	// ImagePushSecrets holds references to secrets and their associated registry prefixes.
	ImagePushSecrets []ImagePushSecret `json:"imagePushSecrets,omitempty"`
}

// ImagePushSecret defines a secret used for pushing images, along with the registry prefix it applies to.
type ImagePushSecret struct {
	// Name is the name of the Kubernetes Secret containing registry credentials.
	Name string `json:"name"`
	// Prefix specifies the registry domain this secret applies to (e.g., ghcr.io).
	Prefix string `json:"prefix"`
}

// DataPlaneSpec defines the desired state of DataPlane.
type DataPlaneSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Registry holds configuration for authenticating to container registries used by this DataPlane.
	Registry Registry `json:"registry"`
	// KubernetesCluster specifies the target cluster configuration
	KubernetesCluster KubernetesClusterSpec `json:"kubernetesCluster"`
	// Gateway specifies the gateway configuration
	Gateway GatewaySpec `json:"gateway"`
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
