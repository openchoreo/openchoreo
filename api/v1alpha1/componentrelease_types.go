// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ComponentReleaseSpec defines the desired state of ComponentRelease.
type ComponentReleaseSpec struct {
	// Owner identifies the component and project this ComponentRelease belongs to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.owner is immutable"
	Owner ComponentReleaseOwner `json:"owner"`

	// ComponentType is a full embedded copy of the ComponentType
	// This ensures the ComponentRelease has an immutable snapshot of the ComponentType at the time of component release
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.componentType is immutable"
	ComponentType ComponentTypeSpec `json:"componentType"`

	// Traits holds frozen trait specifications at the time of ComponentRelease, ensuring immutability.
	// Each entry carries its kind (Trait or ClusterTrait), name, and spec, preserving the original
	// resource identity so that a Trait and ClusterTrait with the same name can coexist.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.traits is immutable"
	Traits []ComponentReleaseTrait `json:"traits,omitempty"`

	// ComponentProfile contains the immutable snapshot of parameter values and trait configs
	// specified for this component at release time
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.componentProfile is immutable"
	ComponentProfile *ComponentProfile `json:"componentProfile,omitempty"`

	// Workload is a full embedded copy of the Workload
	// This preserves the workload spec with the built image
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.workload is immutable"
	Workload WorkloadTemplateSpec `json:"workload"`
}

// ComponentReleaseTrait is an entry in the frozen traits snapshot stored on a ComponentRelease.
// It preserves both the Kind and Name of the original trait resource so that a namespace-scoped
// Trait and a cluster-scoped ClusterTrait with the same name can coexist.
type ComponentReleaseTrait struct {
	// Kind identifies whether this is a namespace-scoped Trait or a cluster-scoped ClusterTrait.
	// +kubebuilder:validation:Required
	Kind TraitRefKind `json:"kind"`

	// Name is the name of the Trait or ClusterTrait resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Spec is the frozen specification of the trait at the time of this ComponentRelease.
	// +kubebuilder:validation:Required
	Spec TraitSpec `json:"spec"`
}

// ComponentProfile defines a snapshot of a component's spec
type ComponentProfile struct {
	// Parameters holds the snapshot of parameter values from the Component spec
	// The schema for these values is defined in the ComponentType's parameters schema
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`

	// Traits holds the snapshot of trait instances configured on the component at release time.
	// Each entry records the kind, name, and instanceName of the trait, along with any
	// user-supplied parameters, using the composite (kind, name) key to unambiguously identify
	// the trait spec in ComponentReleaseSpec.Traits.
	// +optional
	Traits []ComponentProfileTrait `json:"traits,omitempty"`
}

// ComponentProfileTrait is a snapshot of a single trait instance configured on a component.
// It records the kind and name of the trait (to look up the spec in ComponentReleaseSpec.Traits),
// the instance name (unique within the component), and any user-supplied parameters.
type ComponentProfileTrait struct {
	// Kind identifies whether this is a namespace-scoped Trait or a cluster-scoped ClusterTrait.
	// +kubebuilder:validation:Required
	Kind TraitRefKind `json:"kind"`

	// Name is the name of the Trait or ClusterTrait resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// InstanceName uniquely identifies this trait instance within the component.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	InstanceName string `json:"instanceName"`

	// Parameters contains the trait parameter values supplied by the user.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// ComponentReleaseStatus defines the observed state of ComponentRelease.
type ComponentReleaseStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="Component",type=string,JSONPath=`.spec.owner.componentName`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

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
