// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type WorkloadReleaseOwner struct {
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

// WorkloadReleaseSpec defines the desired state of WorkloadRelease.
type WorkloadReleaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Owner WorkloadReleaseOwner `json:"owner"`
	// +kubebuilder:validation:MinLength=1
	EnvironmentName string `json:"environmentName"`

	Type WorkloadType `json:"type"`

	// Scalable resource template approach (KRO-inspired)
	// Supports any Kubernetes resource type including HPA, PDB, NetworkPolicy, CRDs, etc. that can
	// be applied to the data plane.
	// +kubebuilder:validation:Optional
	Resources []Resource `json:"resources,omitempty"`
}

// WorkloadReleaseStatus defines the observed state of WorkloadRelease.
type WorkloadReleaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// WorkloadRelease is the Schema for the workloadreleases API.
type WorkloadRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadReleaseSpec   `json:"spec,omitempty"`
	Status WorkloadReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkloadReleaseList contains a list of WorkloadRelease.
type WorkloadReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadRelease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkloadRelease{}, &WorkloadReleaseList{})
}
