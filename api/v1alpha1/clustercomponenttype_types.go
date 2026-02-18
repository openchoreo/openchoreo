// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterComponentTypeSpec defines the desired state of ClusterComponentType.
// +kubebuilder:validation:XValidation:rule="self.workloadType == 'proxy' || self.resources.exists(r, r.id == self.workloadType)",message="resources must contain a primary resource with id matching workloadType (unless workloadType is 'proxy')"
type ClusterComponentTypeSpec struct {
	// WorkloadType must be one of: deployment, statefulset, cronjob, job, proxy
	// This determines the primary workload resource type for this component type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=deployment;statefulset;cronjob;job;proxy
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.workloadType cannot be changed after creation"
	WorkloadType string `json:"workloadType"`

	// AllowedWorkflows restricts which ComponentWorkflow CRs developers can use
	// for building components of this type. If empty, no ComponentWorkflows are allowed.
	// References must point to ComponentWorkflow resources, not generic Workflow resources.
	// +optional
	AllowedWorkflows []string `json:"allowedWorkflows,omitempty"`

	// Schema defines what developers can configure when creating components of this type
	// +optional
	Schema ComponentTypeSchema `json:"schema,omitempty"`

	// Traits are pre-configured trait instances embedded in the ComponentType.
	// The PE binds trait parameters using concrete values or CEL expressions
	// referencing the ComponentType schema (e.g., "${parameters.storage.mountPath}").
	// These traits are automatically applied to all Components of this type.
	// +optional
	Traits []ComponentTypeTrait `json:"traits,omitempty"`

	// AllowedTraits restricts which Trait CRs developers can attach to Components of this type.
	// When specified, only traits listed here may be attached beyond those already embedded in spec.traits.
	// Trait names listed here must not overlap with traits already embedded in spec.traits.
	// If empty or omitted, no additional component-level traits are allowed.
	// +optional
	AllowedTraits []string `json:"allowedTraits,omitempty"`

	// Validations are CEL-based rules evaluated during rendering.
	// All rules must evaluate to true for rendering to proceed.
	// +optional
	Validations []ValidationRule `json:"validations,omitempty"`

	// Resources are templates that generate Kubernetes resources dynamically.
	// At least one resource template is required. For non-proxy workload types,
	// one resource must have an id matching the workloadType. When workloadType
	// is "proxy", a matching resource id is not required.
	// +kubebuilder:validation:MinItems=1
	Resources []ResourceTemplate `json:"resources"`
}

// ClusterComponentTypeStatus defines the observed state of ClusterComponentType.
type ClusterComponentTypeStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cct;ccts
// +kubebuilder:printcolumn:name="WorkloadType",type=string,JSONPath=`.spec.workloadType`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterComponentType is the Schema for the clustercomponenttypes API.
// ClusterComponentType is a cluster-scoped version of ComponentType that can be
// referenced by Components across all namespaces.
type ClusterComponentType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterComponentTypeSpec   `json:"spec,omitempty"`
	Status ClusterComponentTypeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterComponentTypeList contains a list of ClusterComponentType.
type ClusterComponentTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterComponentType `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterComponentType{}, &ClusterComponentTypeList{})
}
