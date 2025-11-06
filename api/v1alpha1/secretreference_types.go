// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SecretTemplate defines the structure of the resulting Kubernetes Secret.
type SecretTemplate struct {
	// Type of the Kubernetes Secret (Opaque, kubernetes.io/dockerconfigjson, etc.)
	// +kubebuilder:validation:Enum=Opaque;kubernetes.io/dockerconfigjson;kubernetes.io/dockercfg;kubernetes.io/basic-auth;kubernetes.io/ssh-auth;kubernetes.io/tls;bootstrap.kubernetes.io/token
	// +kubebuilder:default="Opaque"
	// +optional
	Type corev1.SecretType `json:"type,omitempty"`

	// Metadata to add to the generated secret
	// +optional
	Metadata *SecretMetadata `json:"metadata,omitempty"`
}

// SecretMetadata defines additional metadata for the generated secret.
type SecretMetadata struct {
	// Annotations to add to the secret
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels to add to the secret
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// SecretDataSource maps a secret key to an external secret reference.
type SecretDataSource struct {
	// SecretKey is the key name in the Kubernetes Secret
	// +kubebuilder:validation:MinLength=1
	SecretKey string `json:"secretKey"`

	// RemoteRef points to the external secret location
	RemoteRef RemoteReference `json:"remoteRef"`
}

// RemoteReference points to a secret in an external secret store.
type RemoteReference struct {
	// Key is the path in the external secret store (e.g., "secret/data/github/pat")
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`

	// Property is the specific field within the secret (e.g., "token")
	// +optional
	Property string `json:"property,omitempty"`

	// Version of the secret to fetch (provider-specific)
	// +optional
	Version string `json:"version,omitempty"`
}

// SecretReferenceSpec defines the desired state of SecretReference.
type SecretReferenceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Template defines the structure of the resulting Kubernetes Secret
	Template SecretTemplate `json:"template"`

	// Data contains the mapping of secret keys to external secret references
	// +kubebuilder:validation:MinItems=1
	Data []SecretDataSource `json:"data"`

	// RefreshInterval specifies how often to reconcile/refresh the secret
	// +optional
	// +kubebuilder:default="1h"
	RefreshInterval *metav1.Duration `json:"refreshInterval,omitempty"`
}

// SecretStoreReference tracks where this SecretReference is being used.
type SecretStoreReference struct {
	// Name of the secret store
	Name string `json:"name"`

	// Namespace where the ExternalSecret was created
	Namespace string `json:"namespace"`

	// Kind of resource (ExternalSecret, ClusterExternalSecret)
	Kind string `json:"kind"`
}

// SecretReferenceStatus defines the observed state of SecretReference.
type SecretReferenceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastRefreshTime indicates when the secret reference was last processed
	// +optional
	LastRefreshTime *metav1.Time `json:"lastRefreshTime,omitempty"`

	// SecretStores tracks which secret stores are using this reference
	// +optional
	SecretStores []SecretStoreReference `json:"secretStores,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// SecretReference is the Schema for the secretreferences API.
type SecretReference struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecretReferenceSpec   `json:"spec,omitempty"`
	Status SecretReferenceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SecretReferenceList contains a list of SecretReference.
type SecretReferenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecretReference `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecretReference{}, &SecretReferenceList{})
}
