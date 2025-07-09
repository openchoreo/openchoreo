// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GitCommitRequestSpec defines the desired state of GitCommitRequest.
type GitCommitRequestSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// HTTPS or SSH URL of the repo, e.g. https://github.com/org/repo.git
	RepoURL string `json:"repoURL"`
	// Branch to commit into
	Branch string `json:"branch,omitempty"`
	// The commit message
	Message string `json:"message"`
	// Author information for the commit
	Author GitCommitAuthor `json:"author,omitempty"`
	// Reference to a Secret that contains write credentials
	// data["username"], data["password"] for HTTPS  **or**
	// data["ssh-privatekey"] for SSH
	AuthSecretRef string `json:"authSecretRef,omitempty"`
	// Files to create or patch
	Files []FileEdit `json:"files"`
}

type GitCommitAuthor struct {
	Name  string `json:"name,omitempty"`  // Author name
	Email string `json:"email,omitempty"` // Author email
}

type FileEdit struct {
	Path    string `json:"path"`              // path inside repo
	Content string `json:"content,omitempty"` // full replacement
	Patch   string `json:"patch,omitempty"`   // optional RFC-6902 JSON patch
}

// GitCommitRequestStatus defines the observed state of GitCommitRequest.
type GitCommitRequestStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Phase          string `json:"phase,omitempty"`          // Pending|Succeeded|Failed
	ObservedSHA    string `json:"observedSHA,omitempty"`    // last commit SHA
	ObservedBranch string `json:"observedBranch,omitempty"` // branch we pushed
	Message        string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// GitCommitRequest is the Schema for the gitcommitrequests API.
type GitCommitRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitCommitRequestSpec   `json:"spec,omitempty"`
	Status GitCommitRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitCommitRequestList contains a list of GitCommitRequest.
type GitCommitRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitCommitRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitCommitRequest{}, &GitCommitRequestList{})
}
