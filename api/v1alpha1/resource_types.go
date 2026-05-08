// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ResourceSpec defines the desired state of Resource.
type ResourceSpec struct {
	// Owner identifies the Project this Resource belongs to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.owner is immutable"
	Owner ResourceOwner `json:"owner"`

	// Type references the ResourceType or ClusterResourceType template for this Resource.
	// Kind defaults to ResourceType (namespaced); ClusterResourceType is also allowed.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.type cannot be changed after creation"
	Type ResourceTypeRef `json:"type"`

	// Parameters contains values for the parameter schema declared on the referenced
	// ResourceType (or ClusterResourceType). Validated against the schema by the
	// Resource controller; failures surface via status.conditions.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// ResourceOwner identifies the project that owns a Resource.
type ResourceOwner struct {
	// ProjectName is the name of the Project this Resource belongs to.
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
}

// ResourceStatus defines the observed state of Resource.
type ResourceStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the Resource's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LatestRelease is the most recent ResourceRelease for this Resource.
	// +optional
	LatestRelease *LatestResourceRelease `json:"latestRelease,omitempty"`
}

// LatestResourceRelease identifies the most recent ResourceRelease for a Resource.
// Distinct from LatestRelease (component_types.go) because the hash semantics differ:
// here it covers Resource.spec + ResourceType/ClusterResourceType.spec.
type LatestResourceRelease struct {
	// Name of the ResourceRelease resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Hash is the content hash of Resource.spec + ResourceType/ClusterResourceType.spec
	// captured at release time.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Hash string `json:"hash"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=res
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type.name`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Resource is the Schema for the resources API.
// Developers create Resource objects to declare a managed-infrastructure dependency
// (database, queue, cache, etc.) by referencing a ResourceType or ClusterResourceType.
type Resource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceSpec   `json:"spec,omitempty"`
	Status ResourceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ResourceList contains a list of Resource.
type ResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Resource `json:"items"`
}

func (r *Resource) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *Resource) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&Resource{}, &ResourceList{})
}
