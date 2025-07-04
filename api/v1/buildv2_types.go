// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Repository defines the source repository configuration
type Repository struct {
	// URL is the repository URL
	URL string `json:"url"`

	// Revision specifies the revision to build from
	Revision Revision `json:"revision"`

	// AppPath is the path to the application within the repository
	AppPath string `json:"appPath"`
}

// Revision defines the revision specification
type Revision struct {
	// Branch specifies the branch to build from
	Branch string `json:"branch,omitempty"`

	// Commit specifies the commit hash to build from
	Commit string `json:"commit,omitempty"`
}

// TemplateRef defines the build template reference
type TemplateRef struct {
	// Engine specifies the build engine
	Engine string `json:"engine,omitempty"`

	// Name is the template name
	Name string `json:"name"`

	// Parameters contains the template parameters
	Parameters []Parameter `json:"parameters,omitempty"`
}

// Parameter defines a template parameter
type Parameter struct {
	// Name is the parameter name
	Name string `json:"name"`

	// Value is the parameter value
	Value string `json:"value"`
}

type BuildOwner struct {
	// +kubebuilder:validation:MinLength=1
	OrganizationName string `json:"organizationName"`
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// BuildV2Spec defines the desired state of BuildV2.
type BuildV2Spec struct {
	Owner BuildOwner `json:"owner"`

	// Repository contains the source repository configuration
	Repository Repository `json:"repository"`

	// TemplateRef contains the build template reference and parameters
	TemplateRef TemplateRef `json:"templateRef"`
}

// BuildV2Status defines the observed state of BuildV2.
type BuildV2Status struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// BuildV2 is the Schema for the buildv2s API.
type BuildV2 struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuildV2Spec   `json:"spec,omitempty"`
	Status BuildV2Status `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BuildV2List contains a list of BuildV2.
type BuildV2List struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BuildV2 `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BuildV2{}, &BuildV2List{})
}
