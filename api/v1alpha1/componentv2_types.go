// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ComponentOwner struct {
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
}

// ComponentV2Spec defines the desired state of ComponentV2.
type ComponentV2Spec struct {
	// Owner defines the ownership information for the component
	Owner ComponentOwner `json:"owner"`

	// Type specifies the component type (e.g., Service, WebApplication, etc.)
	Type ComponentType `json:"type"`

	// Build defines the build configuration for the component
	Build BuildSpecInComponent `json:"build,omitempty"`
}

// BuildSpecInComponent defines the build configuration for a component
// This specification is used to create BuildV2 resources when builds are triggered
type BuildSpecInComponent struct {
	// Repository defines the source repository configuration where the component code resides
	Repository BuildRepository `json:"repository"`

	// TemplateRef defines the build template reference and configuration
	// This references a ClusterWorkflowTemplate in the build plane
	TemplateRef TemplateRef `json:"templateRef"`
}

// BuildRepository defines the source repository configuration for component builds
type BuildRepository struct {
	// URL is the repository URL where the component source code is located
	// Example: "https://github.com/org/repo" or "git@github.com:org/repo.git"
	URL string `json:"url"`

	// Revision specifies the default revision configuration for builds
	// This can be overridden when triggering specific builds
	Revision BuildRevision `json:"revision"`

	// AppPath is the path to the application within the repository
	// This is relative to the repository root. Default is "." for root directory
	AppPath string `json:"appPath"`
}

// BuildRevision defines the revision specification for component builds
type BuildRevision struct {
	// Branch specifies the default branch to build from
	// This will be used when no specific commit is provided for a build
	Branch string `json:"branch"`
}

// ComponentV2Status defines the observed state of ComponentV2.
type ComponentV2Status struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ComponentV2 is the Schema for the componentv2s API.
type ComponentV2 struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentV2Spec   `json:"spec,omitempty"`
	Status ComponentV2Status `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComponentV2List contains a list of ComponentV2.
type ComponentV2List struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentV2 `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComponentV2{}, &ComponentV2List{})
}
