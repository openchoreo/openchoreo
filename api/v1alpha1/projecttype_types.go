// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectTypeSpec defines the desired state of ProjectType.
type ProjectTypeSpec struct {
	// Parameters is the schema for Project.spec.parameters values supplied by
	// Project authors. Validated against this schema.
	// +optional
	Parameters *SchemaSection `json:"parameters,omitempty"`

	// EnvironmentConfigs defines the per-env schema.
	// Validates ProjectReleaseBinding.spec.environmentConfigs.
	// +optional
	EnvironmentConfigs *SchemaSection `json:"environmentConfigs,omitempty"`

	// Validations are CEL-based rules evaluated during rendering.
	// All rules must evaluate to true for rendering to proceed.
	// +optional
	Validations []ValidationRule `json:"validations,omitempty"`

	// Resources are templates that generate namespace-scoped Kubernetes
	// manifests applied to the cell namespace owned by every
	// ProjectReleaseBinding of this type. Each entry has a unique id;
	// includeWhen and forEach control conditional and iterative rendering;
	// CEL expressions in the template body are evaluated against
	// ${parameters.*}, ${environmentConfigs.*}, and ${metadata.*}.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=id
	Resources []ResourceTemplate `json:"resources"`
}

// ProjectTypeStatus defines the observed state of ProjectType.
type ProjectTypeStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=pt;pts
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ProjectType is the Schema for the projecttypes API.
// PEs publish ProjectType templates in a namespace; developers reference them
// by name from Project.spec.type.
type ProjectType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectTypeSpec   `json:"spec,omitempty"`
	Status ProjectTypeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProjectTypeList contains a list of ProjectType.
type ProjectTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProjectType `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProjectType{}, &ProjectTypeList{})
}
