// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterProjectTypeSpec defines the desired state of ClusterProjectType.
// Currently mirrors ProjectTypeSpec; may diverge as cluster-scoped concerns emerge.
type ClusterProjectTypeSpec struct {
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

// ClusterProjectTypeStatus defines the observed state of ClusterProjectType.
type ClusterProjectTypeStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cpt;cpts
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterProjectType is the Schema for the clusterprojecttypes API.
// ClusterProjectType is the cluster-scoped sibling of ProjectType. Projects in
// any namespace can reference a ClusterProjectType via Project.spec.type.
type ClusterProjectType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterProjectTypeSpec   `json:"spec,omitempty"`
	Status ClusterProjectTypeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterProjectTypeList contains a list of ClusterProjectType.
type ClusterProjectTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterProjectType `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterProjectType{}, &ClusterProjectTypeList{})
}
