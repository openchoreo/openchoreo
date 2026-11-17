// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReleaseBindingRevisionSpec defines the recorded state of one ReleaseBinding revision.
// The spec is fixed after creation; only status may change.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="revision spec is immutable; only status may change"
type ReleaseBindingRevisionSpec struct {
	// Revision is the sequence number of this revision within its ReleaseBinding.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	Revision int64 `json:"revision"`

	// Cause records why this revision was created.
	// +kubebuilder:validation:Required
	Cause RevisionCause `json:"cause"`

	// ComponentReleaseName is the name of the ComponentRelease bound by this revision.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ComponentReleaseName string `json:"componentReleaseName"`

	// SnapshotHash is the hash of the binding spec captured in this revision.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SnapshotHash string `json:"snapshotHash"`

	// Message is the optional reason given for the change this revision records,
	// copied from the binding's openchoreo.dev/change-message annotation.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Message string `json:"message,omitempty"`

	// BindingSpec is a copy of the ReleaseBinding spec at the time of this revision.
	// +kubebuilder:validation:Required
	BindingSpec ReleaseBindingSpec `json:"bindingSpec"`
}

// BindingRevisionStatus defines the observed state of a ReleaseBindingRevision or
// ResourceReleaseBindingRevision. It is written by the controller and keeps changing
// after the revision is created.
type BindingRevisionStatus struct {
	// Health is the observed health of this revision.
	// +optional
	Health RevisionHealth `json:"health,omitempty"`

	// Active is true when this revision is the one currently applied by its binding.
	// +optional
	Active bool `json:"active,omitempty"`

	// KnownGood is true once this revision has been observed healthy.
	// +optional
	KnownGood bool `json:"knownGood,omitempty"`

	// ObservedAt is when the controller last observed this revision's health.
	// +optional
	ObservedAt *metav1.Time `json:"observedAt,omitempty"`

	// HealthySince is when this revision last became healthy.
	// +optional
	HealthySince *metav1.Time `json:"healthySince,omitempty"`

	// Conditions represent the latest available observations of the revision's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=rbr
// +kubebuilder:printcolumn:name="Revision",type=integer,JSONPath=`.spec.revision`
// +kubebuilder:printcolumn:name="Release",type=string,JSONPath=`.spec.componentReleaseName`
// +kubebuilder:printcolumn:name="Cause",type=string,JSONPath=`.spec.cause`
// +kubebuilder:printcolumn:name="Health",type=string,JSONPath=`.status.health`
// +kubebuilder:printcolumn:name="Active",type=boolean,JSONPath=`.status.active`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ReleaseBindingRevision is the Schema for the releasebindingrevisions API.
// Records one revision of a ReleaseBinding; owned by that ReleaseBinding.
type ReleaseBindingRevision struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReleaseBindingRevisionSpec `json:"spec,omitempty"`
	Status BindingRevisionStatus      `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ReleaseBindingRevisionList contains a list of ReleaseBindingRevision.
type ReleaseBindingRevisionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReleaseBindingRevision `json:"items"`
}

// GetConditions returns the conditions from the status.
func (r *ReleaseBindingRevision) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

// SetConditions sets the conditions in the status.
func (r *ReleaseBindingRevision) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&ReleaseBindingRevision{}, &ReleaseBindingRevisionList{})
}
