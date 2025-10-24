// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ComponentEnvSnapshotSpec defines the desired state of ComponentEnvSnapshot.
type ComponentEnvSnapshotSpec struct {
	// Owner identifies the component and environment this snapshot belongs to
	// +kubebuilder:validation:Required
	Owner ComponentEnvSnapshotOwner `json:"owner"`

	// Environment is the name of the environment this snapshot is for
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Environment string `json:"environment"`

	// ComponentTypeDefinition is a full embedded copy of the ComponentTypeDefinition
	// This ensures the snapshot is immutable and doesn't change if the CTD is updated
	// +kubebuilder:validation:Required
	ComponentTypeDefinition ComponentTypeDefinition `json:"componentTypeDefinition"`

	// Component is a full embedded copy of the Component
	// This preserves the exact component configuration at the time of snapshot
	// +kubebuilder:validation:Required
	Component Component `json:"component"`

	// Addons is an array of full embedded copies of all Addons used by the component
	// This preserves the exact addon definitions at the time of snapshot
	// +optional
	Addons []Addon `json:"addons,omitempty"`

	// Workload is a full embedded copy of the Workload
	// This preserves the workload spec with the built image
	// +kubebuilder:validation:Required
	Workload Workload `json:"workload"`
}

// ComponentEnvSnapshotOwner identifies the component this snapshot belongs to
type ComponentEnvSnapshotOwner struct {
	// ProjectName is the name of the project that owns this component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// ComponentName is the name of the component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// ComponentEnvSnapshotStatus defines the observed state of ComponentEnvSnapshot.
type ComponentEnvSnapshotStatus struct {
	// ObservedGeneration reflects the generation of the most recently observed ComponentEnvSnapshot
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the ComponentEnvSnapshot's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ReleaseRef points to the Release resource generated from this snapshot
	// +optional
	ReleaseRef string `json:"releaseRef,omitempty"`

	// RenderedResourceCount is the number of Kubernetes resources rendered from this snapshot
	// +optional
	RenderedResourceCount int `json:"renderedResourceCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=snapshot;snapshots
// +kubebuilder:printcolumn:name="Component",type=string,JSONPath=`.spec.owner.componentName`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="Release",type=string,JSONPath=`.status.releaseRef`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ComponentEnvSnapshot is the Schema for the componentenvsnapshots API.
type ComponentEnvSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentEnvSnapshotSpec   `json:"spec,omitempty"`
	Status ComponentEnvSnapshotStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComponentEnvSnapshotList contains a list of ComponentEnvSnapshot.
type ComponentEnvSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentEnvSnapshot `json:"items"`
}

func (c *ComponentEnvSnapshot) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

func (c *ComponentEnvSnapshot) SetConditions(conditions []metav1.Condition) {
	c.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&ComponentEnvSnapshot{}, &ComponentEnvSnapshotList{})
}
