// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ValueFrom defines a common pattern for referencing secrets or providing inline values
type ValueFrom struct {
	// SecretRef is a reference to a secret containing the value
	// +optional
	SecretRef string `json:"secretRef,omitempty"`
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

// BasicAuthCredentials defines username and password for basic authentication
type BasicAuthCredentials struct {
	// Username for basic authentication
	Username string `json:"username"`
	// Password for basic authentication
	Password string `json:"password"`
}

// ObserverAuthentication defines authentication configuration for Observer API
type ObserverAuthentication struct {
	// BasicAuth contains basic authentication credentials
	BasicAuth BasicAuthCredentials `json:"basicAuth"`
}

// ObserverAPI defines the configuration for the Observer API integration
type ObserverAPI struct {
	// URL is the base URL of the Observer API
	URL string `json:"url"`
	// Authentication contains the authentication configuration
	Authentication ObserverAuthentication `json:"authentication"`
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

	// ImagePullSecretRefs contains references to SecretReference resources
	// These will be converted to ExternalSecrets and added as imagePullSecrets to all deployments
	// +optional
	ImagePullSecretRefs []string `json:"imagePullSecretRefs,omitempty"`

	// SecretStoreRef specifies the ESO ClusterSecretStore to use in the data plane
	// +optional
	SecretStoreRef *SecretStoreRef `json:"secretStoreRef,omitempty"`

	// KubernetesCluster defines the target Kubernetes cluster where workloads should be deployed.
	KubernetesCluster KubernetesClusterSpec `json:"kubernetesCluster"`
	// Gateway specifies the configuration for the API gateway in this DataPlane.
	Gateway GatewaySpec `json:"gateway"`
	// Observer specifies the configuration for the Observer API integration.
	// +optional
	Observer ObserverAPI `json:"observer,omitempty"`
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
