// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ReleaseLineSpec defines the desired state of ReleaseLine.
// Values set here are layered on top of the owning Component's spec for this line only.
type ReleaseLineSpec struct {
	// Owner identifies the component this release line belongs to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.owner is immutable"
	Owner ReleaseLineOwner `json:"owner"`

	// AutoDeploy overrides Component.spec.autoDeploy for this line.
	// +optional
	AutoDeploy *bool `json:"autoDeploy,omitempty"`

	// Parameters are deep-merged onto Component.spec.parameters for this line.
	// Maps merge key by key; scalars and lists replace.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`

	// Traits are merged onto Component.spec.traits for this line, keyed by (kind, name, instanceName).
	// A matching trait has its parameters deep-merged; a new trait is appended.
	// +optional
	Traits []ComponentTrait `json:"traits,omitempty"`

	// Workflow overrides the build configuration of Component.spec.workflow for this line.
	// +optional
	Workflow *ReleaseLineWorkflow `json:"workflow,omitempty"`
}

// ReleaseLineWorkflow holds a line's overrides of the component's workflow.
// The workflow kind and name are shared by every line.
type ReleaseLineWorkflow struct {
	// Parameters are deep-merged onto Component.spec.workflow.parameters for this line,
	// typically to build from a different branch.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// ReleaseLineOwner identifies the component a ReleaseLine belongs to.
type ReleaseLineOwner struct {
	// ProjectName is the name of the project that owns the component.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// ComponentName is the name of the component.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// ReleaseLineStatus defines the observed state of ReleaseLine.
type ReleaseLineStatus struct {
	// ObservedGeneration is the most recent generation observed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the release line.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LatestRelease is the most recent ComponentRelease produced for this line by autoDeploy.
	// +optional
	LatestRelease *LatestRelease `json:"latestRelease,omitempty"`

	// Environments lists the line's ReleaseBindings, one per environment it is deployed to.
	// +optional
	// +listType=map
	// +listMapKey=environment
	Environments []ReleaseLineEnvironment `json:"environments,omitempty"`
}

// ReleaseLineEnvironment is a release line's deployment in one environment.
type ReleaseLineEnvironment struct {
	// Environment is the name of the environment.
	Environment string `json:"environment"`

	// ReleaseBinding is the name of the line's ReleaseBinding in the environment.
	ReleaseBinding string `json:"releaseBinding"`

	// Release is the ComponentRelease bound in the environment.
	// +optional
	Release string `json:"release,omitempty"`

	// Ready reports whether the ReleaseBinding is Ready.
	// +optional
	Ready bool `json:"ready,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=rl;rls
// +kubebuilder:selectablefield:JSONPath=`.spec.owner.projectName`
// +kubebuilder:selectablefield:JSONPath=`.spec.owner.componentName`
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="Component",type=string,JSONPath=`.spec.owner.componentName`
// +kubebuilder:printcolumn:name="LatestRelease",type=string,JSONPath=`.status.latestRelease.name`
// +kubebuilder:printcolumn:name="Environments",type=string,JSONPath=`.status.environments[*].environment`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:validation:XValidation:rule="size(self.metadata.name) <= 63",message="metadata.name must be at most 63 characters"

// ReleaseLine is a named series of releases of one Component that evolves independently of the
// component's other lines. Each line has its own Workload, ComponentReleases, and at most one
// ReleaseBinding per Environment. The owning Component must have spec.releaseLinesEnabled set;
// such a component has no implicit line, so its default line (e.g. "main") is an explicit ReleaseLine.
type ReleaseLine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReleaseLineSpec   `json:"spec,omitempty"`
	Status ReleaseLineStatus `json:"status,omitempty"`
}

// GetConditions returns the conditions of the ReleaseLine.
func (r *ReleaseLine) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

// SetConditions sets the conditions of the ReleaseLine.
func (r *ReleaseLine) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// ReleaseLineList contains a list of ReleaseLine.
type ReleaseLineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReleaseLine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ReleaseLine{}, &ReleaseLineList{})
}
