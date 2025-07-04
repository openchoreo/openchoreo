// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ReleaseSpec defines the desired state of Release.
type ReleaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Owner ReleaseOwner `json:"owner"`
	// +kubebuilder:validation:MinLength=1
	EnvironmentName string `json:"environmentName"`

	// Scalable resource template approach (KRO-inspired)
	// Supports any Kubernetes resource type including HPA, PDB, NetworkPolicy, CRDs, etc. that can
	// be applied to the data plane.
	// +kubebuilder:validation:Optional
	Resources []Resource `json:"resources,omitempty"`

	// Interval watch interval for the release resources when stable.
	// Defaults to 5m if not specified.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ms|s|m|h))+$"
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`

	// ProgressingInterval watch interval for the release resources when transitioning.
	// Defaults to 10s if not specified.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ms|s|m|h))+$"
	// +optional
	ProgressingInterval *metav1.Duration `json:"progressingInterval,omitempty"`
}

// ReleaseStatus defines the observed state of Release.
type ReleaseStatus struct {
	// Resources contain the list of resources that have been successfully applied to the data plane
	// +optional
	Resources []ResourceStatus `json:"resources,omitempty"`

	// Conditions represent the latest available observations of the Release's current state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Release is the Schema for the releases API.
type Release struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReleaseSpec   `json:"spec,omitempty"`
	Status ReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ReleaseList contains a list of Release.
type ReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Release `json:"items"`
}

// ReleaseOwner defines the owner of a Release.
type ReleaseOwner struct {
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// Resource defines a Kubernetes resource template that can be applied to the data plane.
type Resource struct {
	// Unique identifier for the resource
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// Object contains the complete Kubernetes resource definition
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Object *runtime.RawExtension `json:"object"`
}

// ResourceStatus tracks a resource that was applied to the data plane.
type ResourceStatus struct {
	// ID corresponds to the resource ID in spec.resources
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// Group is the API group of the resource (e.g., "apps", "batch")
	// Empty string for core resources
	// +optional
	Group string `json:"group,omitempty"`

	// Version is the API version of the resource (e.g., "v1", "v1beta1")
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

	// Kind is the type of the resource (e.g., "Deployment", "Service")
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`

	// Name is the name of the resource in the data plane
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace is the namespace of the resource in the data plane
	// Empty for cluster-scoped resources
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Status captures the entire .status field of the resource applied to the data plane.
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Status *runtime.RawExtension `json:"status,omitempty"`

	// LastObservedTime stores the last time the status was observed
	// +optional
	LastObservedTime *metav1.Time `json:"lastObservedTime,omitempty"`
}

// GetConditions returns the conditions from the status
func (in *Release) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets the conditions in the status
func (in *Release) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&Release{}, &ReleaseList{})
}
