// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AuthType represents the authentication method for Kubernetes cluster access
// +kubebuilder:validation:Enum=cert;bearer;oidc
type AuthType string

const (
	// AuthTypeCert represents certificate-based authentication (mTLS)
	AuthTypeCert AuthType = "cert"
	// AuthTypeBearer represents bearer token authentication
	AuthTypeBearer AuthType = "bearer"
	// AuthTypeOIDC represents OIDC authentication
	AuthTypeOIDC AuthType = "oidc"
)

// KubernetesClusterSpec defines the configuration for the target Kubernetes cluster
type KubernetesClusterSpec struct {
	// Connection contains the server connection details
	Connection KubernetesConnection `json:"connection"`
	// Auth contains the authentication configuration
	Auth KubernetesAuth `json:"auth"`
}

// KubernetesConnection defines the connection details for the Kubernetes API server
type KubernetesConnection struct {
	// Server is the URL of the Kubernetes API server
	Server string `json:"server"`
	// TLS contains the TLS configuration for the connection
	TLS KubernetesTLS `json:"tls"`
}

// KubernetesTLS defines the TLS configuration for the Kubernetes connection
type KubernetesTLS struct {
	// CASecretRef is a reference to a secret containing the CA certificate
	// +optional
	CASecretRef string `json:"caSecretRef,omitempty"`
	// CAData is the base64-encoded CA certificate (fallback option)
	// +optional
	CAData string `json:"caData,omitempty"`
}

// KubernetesAuth defines the authentication configuration for the Kubernetes cluster
type KubernetesAuth struct {
	// Type specifies the authentication method: cert, bearer, or oidc
	// +kubebuilder:validation:Enum=cert;bearer;oidc
	Type AuthType `json:"type"`
	// Cert contains the certificate-based authentication configuration
	// +optional
	Cert *KubernetesCertAuth `json:"cert,omitempty"`
	// Bearer contains the bearer token authentication configuration
	// +optional
	Bearer *KubernetesBearerAuth `json:"bearer,omitempty"`
	// OIDC contains the OIDC authentication configuration
	// +optional
	OIDC *KubernetesOIDCAuth `json:"oidc,omitempty"`
}

// KubernetesCertAuth defines certificate-based authentication (mTLS)
type KubernetesCertAuth struct {
	// ClientCertSecretRef is a reference to a secret containing the client certificate
	ClientCertSecretRef string `json:"clientCertSecretRef"`
	// ClientKeySecretRef is a reference to a secret containing the client private key
	ClientKeySecretRef string `json:"clientKeySecretRef"`
	// ClientCertData is the base64-encoded client certificate (fallback option)
	// +optional
	ClientCertData string `json:"clientCertData,omitempty"`
	// ClientKeyData is the base64-encoded client private key (fallback option)
	// +optional
	ClientKeyData string `json:"clientKeyData,omitempty"`
}

// KubernetesBearerAuth defines bearer token authentication
type KubernetesBearerAuth struct {
	// TokenSecretRef is a reference to a secret containing the bearer token
	TokenSecretRef string `json:"tokenSecretRef"`
	// TokenData is the raw token string (fallback option)
	// +optional
	TokenData string `json:"tokenData,omitempty"`
}

// KubernetesOIDCAuth defines OIDC authentication configuration
type KubernetesOIDCAuth struct {
	// Configuration map for OIDC settings
	// This is a placeholder for OIDC configuration which can be expanded based on requirements
	Config map[string]string `json:"config,omitempty"`
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
