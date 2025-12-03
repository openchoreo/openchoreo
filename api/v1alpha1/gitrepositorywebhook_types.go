// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=gitwebhook;gitwebhooks

// GitRepositoryWebhook is the Schema for tracking git repository webhooks
// This resource tracks webhooks at the repository level to support multiple
// components pointing to the same repository
type GitRepositoryWebhook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitRepositoryWebhookSpec   `json:"spec,omitempty"`
	Status GitRepositoryWebhookStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitRepositoryWebhookList contains a list of GitRepositoryWebhook
type GitRepositoryWebhookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitRepositoryWebhook `json:"items"`
}

// GitRepositoryWebhookSpec defines the desired state of GitRepositoryWebhook
type GitRepositoryWebhookSpec struct {
	// RepositoryURL is the normalized URL of the git repository
	// +kubebuilder:validation:Required
	RepositoryURL string `json:"repositoryURL"`

	// Provider is the git provider type (github, gitlab, bitbucket, etc.)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=github;gitlab;bitbucket
	Provider string `json:"provider"`

	// WebhookID is the ID of the webhook in the git provider
	// +optional
	WebhookID string `json:"webhookID,omitempty"`

	// ComponentReferences is a list of components that use this webhook
	// This is used for reference counting
	// +optional
	ComponentReferences []ComponentReference `json:"componentReferences,omitempty"`

	// WebhookSecretRef is a reference to a Kubernetes Secret containing the webhook secret
	// The secret is auto-generated when the webhook is created
	// +optional
	WebhookSecretRef *corev1.SecretReference `json:"webhookSecretRef,omitempty"`
}

// ComponentReference represents a reference to a component
type ComponentReference struct {
	// Namespace is the namespace of the component
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// Name is the name of the component
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// OrgName is the organization name
	// +optional
	OrgName string `json:"orgName,omitempty"`

	// ProjectName is the project name
	// +optional
	ProjectName string `json:"projectName,omitempty"`
}

// GitRepositoryWebhookStatus defines the observed state of GitRepositoryWebhook
type GitRepositoryWebhookStatus struct {
	// Registered indicates if the webhook is successfully registered with the git provider
	// +optional
	Registered bool `json:"registered,omitempty"`

	// ReferenceCount is the number of components using this webhook
	// +optional
	ReferenceCount int `json:"referenceCount,omitempty"`

	// LastSyncTime is the last time the webhook was synced with the git provider
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions represent the latest available observations of the webhook's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func init() {
	SchemeBuilder.Register(&GitRepositoryWebhook{}, &GitRepositoryWebhookList{})
}
