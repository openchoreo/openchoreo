// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ResourceReleaseSpec defines the desired state of ResourceRelease.
// A ResourceRelease is an immutable snapshot of Resource.spec and the referenced
// ResourceType (or ClusterResourceType) spec at the time it was cut. Created
// exclusively by the Resource controller; deleted by the Resource finalizer when
// the parent Resource is torn down.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ResourceRelease spec is immutable"
type ResourceReleaseSpec struct {
	// Owner identifies the resource and project this ResourceRelease belongs to.
	// +kubebuilder:validation:Required
	Owner ResourceReleaseOwner `json:"owner"`

	// ResourceType is a frozen snapshot of the ResourceType or ClusterResourceType
	// resource at the time of the release. Records the kind and name of the
	// original resource alongside its full spec so consumers can distinguish
	// namespace-scoped ResourceTypes from cluster-scoped ClusterResourceTypes.
	// +kubebuilder:validation:Required
	ResourceType ResourceReleaseResourceType `json:"resourceType"`

	// Parameters holds the snapshot of parameter values from the Resource spec at
	// release time. The schema for these values is defined by the ResourceType's
	// parameters schema.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// ResourceReleaseResourceType is the frozen snapshot of a ResourceType or
// ClusterResourceType resource stored on a ResourceRelease. Preserves Kind and
// Name of the original resource alongside its full spec so a ResourceType and a
// ClusterResourceType with the same name can coexist.
type ResourceReleaseResourceType struct {
	// Kind identifies whether this is a namespace-scoped ResourceType or a
	// cluster-scoped ClusterResourceType.
	// +kubebuilder:validation:Required
	Kind ResourceTypeRefKind `json:"kind"`

	// Name is the name of the ResourceType or ClusterResourceType resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Spec is the frozen specification of the (Cluster)ResourceType at the time of
	// this ResourceRelease. ClusterResourceTypeSpec currently mirrors
	// ResourceTypeSpec so both kinds are stored under the namespaced type. If
	// ClusterResourceTypeSpec gains cluster-only fields, snapshots from a
	// ClusterResourceType source will lose them; revisit this field then. Mirrors
	// the ComponentReleaseComponentType precedent.
	// +kubebuilder:validation:Required
	Spec ResourceTypeSpec `json:"spec"`
}

// ResourceReleaseOwner identifies the resource this ResourceRelease belongs to.
type ResourceReleaseOwner struct {
	// ProjectName is the name of the project that owns this resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// ResourceName is the name of the Resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ResourceName string `json:"resourceName"`
}

// ResourceReleaseStatus defines the observed state of ResourceRelease.
type ResourceReleaseStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="Resource",type=string,JSONPath=`.spec.owner.resourceName`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ResourceRelease is the Schema for the resourcereleases API. An immutable
// snapshot of a Resource and its referenced (Cluster)ResourceType at a point in
// time. Created by the Resource controller when the hash of Resource.spec +
// (Cluster)ResourceType.spec changes.
type ResourceRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceReleaseSpec   `json:"spec,omitempty"`
	Status ResourceReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ResourceReleaseList contains a list of ResourceRelease.
type ResourceReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceRelease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceRelease{}, &ResourceReleaseList{})
}
