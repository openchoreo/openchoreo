// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ProjectReleaseBindingSpec defines the desired state of ProjectReleaseBinding.
// Pins a ProjectRelease to an Environment and carries per-env overrides for
// the schema declared on the inlined (Cluster)ProjectType environmentConfigs.
// Owns the cell namespace for (Project, Environment); applies the inlined
// (Cluster)ProjectType.spec.resources to that namespace.
// An empty projectRelease pin is seeded once by the Project controller with
// the project's latest release; advancing the pin afterwards is manual
// (e.g. via `occ project promote` or kubectl edit).
type ProjectReleaseBindingSpec struct {
	// Owner identifies the project this ProjectReleaseBinding belongs to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.owner is immutable"
	Owner ProjectReleaseBindingOwner `json:"owner"`

	// Environment is the name of the Environment this binding targets.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.environment is immutable"
	Environment string `json:"environment"`

	// ProjectRelease is the name of the ProjectRelease pinned by this binding.
	// Leave unset to have the Project controller seed it once with the
	// project's latest release; the binding stays pending until the pin is
	// set. A set pin is never touched by controllers — advancing it is left
	// to whoever drives promotion (occ, GitOps, manual kubectl edit).
	// +optional
	ProjectRelease string `json:"projectRelease,omitempty"`

	// EnvironmentConfigs provides per-environment values for the schema declared
	// on the inlined (Cluster)ProjectType environmentConfigs. Validated against
	// the schema on the referenced ProjectRelease by the binding controller;
	// failures surface via status.conditions.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	EnvironmentConfigs *runtime.RawExtension `json:"environmentConfigs,omitempty"`
}

// ProjectReleaseBindingOwner identifies the project this ProjectReleaseBinding belongs to.
type ProjectReleaseBindingOwner struct {
	// ProjectName is the name of the project that owns this binding.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
}

// ProjectReleaseBindingStatus defines the observed state of ProjectReleaseBinding.
type ProjectReleaseBindingStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the binding's state.
	// Includes Synced, NamespaceReady, ResourcesReady, and Ready (aggregate).
	// observedGeneration is set per-condition (project convention).
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Namespace is the actual data-plane namespace owned by this binding
	// (dp-{ns}-{project}-{env}-{hash}). Surfaced here for kubectl describe and
	// debugging.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=prb;prbs
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="Release",type=string,JSONPath=`.spec.projectRelease`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ProjectReleaseBinding is the Schema for the projectreleasebindings API.
// Pins a ProjectRelease to an Environment, owns the cell namespace for the
// (Project, Environment) tuple, and applies the inlined (Cluster)ProjectType
// resources to that namespace. Authored externally; the Project controller
// only seeds an empty projectRelease pin and cascades deletion on project
// finalize.
type ProjectReleaseBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectReleaseBindingSpec   `json:"spec,omitempty"`
	Status ProjectReleaseBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProjectReleaseBindingList contains a list of ProjectReleaseBinding.
type ProjectReleaseBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProjectReleaseBinding `json:"items"`
}

// GetConditions returns the conditions from the status.
func (r *ProjectReleaseBinding) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

// SetConditions sets the conditions in the status.
func (r *ProjectReleaseBinding) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&ProjectReleaseBinding{}, &ProjectReleaseBindingList{})
}
