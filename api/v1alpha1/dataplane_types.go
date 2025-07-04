// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

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

// Registry defines the container registry configuration, including the image prefix and optional authentication credentials.
type Registry struct {
	// Prefix specifies the registry domain and namespace (e.g., docker.io/namespace) that this configuration applies to.
	Prefix string `json:"prefix"`
	// SecretRef is the name of the Kubernetes Secret containing credentials for accessing the registry.
	// This field is optional and can be omitted for public or unauthenticated registries.
	SecretRef string `json:"secretRef,omitempty"`
}

// DataPlaneSpec defines the desired state of a DataPlane.
type DataPlaneSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Registry contains the configuration required to pull images from a container registry.
	Registry Registry `json:"registry"`
	// KubernetesCluster defines the target Kubernetes cluster where workloads should be deployed.
	KubernetesCluster KubernetesClusterSpec `json:"kubernetesCluster"`
	// Gateway specifies the configuration for the API gateway in this DataPlane.
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
