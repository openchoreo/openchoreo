// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceReleaseBindingRevisionSpec defines the recorded state of one ResourceReleaseBinding revision.
// The spec is fixed after creation; only status may change.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="revision spec is immutable; only status may change"
type ResourceReleaseBindingRevisionSpec struct {
	// Revision is the sequence number of this revision within its ResourceReleaseBinding.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	Revision int64 `json:"revision"`

	// Cause records why this revision was created.
	// +kubebuilder:validation:Required
	Cause RevisionCause `json:"cause"`

	// ResourceReleaseName is the name of the ResourceRelease pinned by this revision.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ResourceReleaseName string `json:"resourceReleaseName"`

	// SnapshotHash is the hash of the binding spec captured in this revision.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SnapshotHash string `json:"snapshotHash"`

	// Message is the optional reason given for the change this revision records,
	// copied from the binding's openchoreo.dev/change-message annotation.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Message string `json:"message,omitempty"`

	// BindingSpec is a copy of the ResourceReleaseBinding spec at the time of this revision.
	// +kubebuilder:validation:Required
	BindingSpec ResourceReleaseBindingSpec `json:"bindingSpec"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=rrbr
// +kubebuilder:printcolumn:name="Revision",type=integer,JSONPath=`.spec.revision`
// +kubebuilder:printcolumn:name="Release",type=string,JSONPath=`.spec.resourceReleaseName`
// +kubebuilder:printcolumn:name="Cause",type=string,JSONPath=`.spec.cause`
// +kubebuilder:printcolumn:name="Health",type=string,JSONPath=`.status.health`
// +kubebuilder:printcolumn:name="Active",type=boolean,JSONPath=`.status.active`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ResourceReleaseBindingRevision is the Schema for the resourcereleasebindingrevisions API.
// Records one revision of a ResourceReleaseBinding; owned by that ResourceReleaseBinding.
type ResourceReleaseBindingRevision struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceReleaseBindingRevisionSpec `json:"spec,omitempty"`
	Status BindingRevisionStatus              `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ResourceReleaseBindingRevisionList contains a list of ResourceReleaseBindingRevision.
type ResourceReleaseBindingRevisionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceReleaseBindingRevision `json:"items"`
}

// GetConditions returns the conditions from the status.
func (r *ResourceReleaseBindingRevision) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

// SetConditions sets the conditions in the status.
func (r *ResourceReleaseBindingRevision) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&ResourceReleaseBindingRevision{}, &ResourceReleaseBindingRevisionList{})
}
