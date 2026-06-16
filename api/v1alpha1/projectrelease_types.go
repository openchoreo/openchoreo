// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ProjectReleaseSpec defines the desired state of ProjectRelease.
// A ProjectRelease is an immutable snapshot of Project.spec and the referenced
// ProjectType (or ClusterProjectType) spec at the time it was cut. Authored
// externally (kubectl, GitOps, API server, or a future Project controller).
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ProjectRelease spec is immutable"
type ProjectReleaseSpec struct {
	// Owner identifies the project this ProjectRelease belongs to.
	// +kubebuilder:validation:Required
	Owner ProjectReleaseOwner `json:"owner"`

	// ProjectType is a frozen snapshot of the ProjectType or ClusterProjectType
	// resource at the time of the release. Records the kind and name of the
	// original resource alongside its full spec so consumers can distinguish
	// namespace-scoped ProjectTypes from cluster-scoped ClusterProjectTypes.
	// +kubebuilder:validation:Required
	ProjectType ProjectReleaseProjectType `json:"projectType"`

	// Parameters holds the snapshot of parameter values from the Project spec at
	// release time. The schema for these values is defined by the ProjectType's
	// parameters schema.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// ProjectReleaseProjectType is the frozen snapshot of a ProjectType or
// ClusterProjectType resource stored on a ProjectRelease. Preserves Kind and
// Name of the original resource alongside its full spec so a ProjectType and a
// ClusterProjectType with the same name can coexist.
type ProjectReleaseProjectType struct {
	// Kind identifies whether this is a namespace-scoped ProjectType or a
	// cluster-scoped ClusterProjectType.
	// +kubebuilder:validation:Required
	Kind ProjectTypeRefKind `json:"kind"`

	// Name is the name of the ProjectType or ClusterProjectType resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Spec is the frozen specification of the (Cluster)ProjectType at the time
	// of this ProjectRelease. ClusterProjectTypeSpec currently mirrors
	// ProjectTypeSpec so both kinds are stored under the namespaced type. If
	// ClusterProjectTypeSpec gains cluster-only fields, snapshots from a
	// ClusterProjectType source will lose them; revisit this field then. Mirrors
	// the ResourceReleaseResourceType precedent.
	// +kubebuilder:validation:Required
	Spec ProjectTypeSpec `json:"spec"`
}

// ProjectReleaseOwner identifies the project this ProjectRelease belongs to.
type ProjectReleaseOwner struct {
	// ProjectName is the name of the project that owns this release.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
}

// ProjectReleaseStatus defines the observed state of ProjectRelease.
type ProjectReleaseStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ProjectRelease is the Schema for the projectreleases API. An immutable
// snapshot of a Project and its referenced (Cluster)ProjectType at a point in
// time.
type ProjectRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectReleaseSpec   `json:"spec,omitempty"`
	Status ProjectReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProjectReleaseList contains a list of ProjectRelease.
type ProjectReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProjectRelease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProjectRelease{}, &ProjectReleaseList{})
}
