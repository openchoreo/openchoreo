// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ComponentReleaseSpec defines the desired state of ComponentRelease.
type ComponentReleaseSpec struct {
	// Owner identifies the component and project this ComponentRelease belongs to
	// +kubebuilder:validation:Required
	Owner ComponentReleaseOwner `json:"owner"`

	// ComponentType is a full embedded copy of the ComponentType
	// This ensures the ComponentRelease has an immutable snapshot of the ComponentType at the time of component release
	// +kubebuilder:validation:Required
	ComponentType ComponentTypeSpec `json:"componentType"`

	// Traits maps trait names to their frozen specifications
	// at the time of ComponentRelease, ensuring immutability
	// +optional
	Traits map[string]TraitSpec `json:"traits,omitempty"`

	// ComponentProfile contains the immutable snapshot of parameter values and trait configs
	// specified for this component at release time
	// +kubebuilder:validation:Required
	ComponentProfile ComponentProfile `json:"componentProfile"`

	// Workload is a full embedded copy of the Workload
	// This preserves the workload spec with the built image
	// +kubebuilder:validation:Required
	Workload WorkloadTemplateSpec `json:"workload"`
}

// ComponentProfile defines a snapshot of a component's spec
type ComponentProfile struct {
	// Parameters from ComponentType (oneOf schema based on componentType)
	// This is the merged schema of parameters + envOverrides from the ComponentType
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`

	// Traits to compose into this component
	// Each trait can be instantiated multiple times with different instanceNames
	// +optional
	Traits []ComponentTrait `json:"traits,omitempty"`
}

// ComponentReleaseStatus defines the observed state of ComponentRelease.
type ComponentReleaseStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ComponentRelease is the Schema for the componentreleases API.
type ComponentRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentReleaseSpec   `json:"spec,omitempty"`
	Status ComponentReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComponentReleaseList contains a list of ComponentRelease.
type ComponentReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentRelease `json:"items"`
}

// ComponentReleaseOwner identifies the component this ComponentRelease belongs to
type ComponentReleaseOwner struct {
	// ProjectName is the name of the project that owns this component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// ComponentName is the name of the component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

func init() {
	SchemeBuilder.Register(&ComponentRelease{}, &ComponentReleaseList{})
}
