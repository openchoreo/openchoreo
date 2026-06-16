// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ProjectSpec defines the desired state of Project.
type ProjectSpec struct {
	// DeploymentPipelineRef references the DeploymentPipeline that defines the environments
	// and deployment progression for components in this project.
	DeploymentPipelineRef DeploymentPipelineRef `json:"deploymentPipelineRef"`

	// Type references the (Cluster)ProjectType that defines the
	// infrastructure template materialized in each environment's cell
	// namespace. Immutable: changing the type after creation is rejected
	// by webhook-level CEL. The Project controller automatically cuts a
	// new ProjectRelease whenever the inlined (Cluster)ProjectType
	// snapshot or Parameters change.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.type cannot be changed after creation"
	Type ProjectTypeRef `json:"type"`

	// Parameters are the project-level inputs validated against the
	// referenced (Cluster)ProjectType's parameters schema and inlined into
	// each ProjectRelease snapshot.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// ProjectStatus defines the observed state of Project.
type ProjectStatus struct {
	// ObservedGeneration reflects the generation of the most recently observed Project.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the current state of the Project resource.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LatestRelease is the most recent ProjectRelease cut for this Project.
	// The Project controller maintains this; ProjectReleaseBindings pin
	// spec.projectRelease to a value here (or to an older release for
	// rollback).
	// +optional
	LatestRelease *LatestProjectRelease `json:"latestRelease,omitempty"`
}

// LatestProjectRelease identifies the most recent ProjectRelease for a Project.
type LatestProjectRelease struct {
	// Name is the name of the ProjectRelease.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Hash is the spec hash that produced the release. The Project
	// controller cuts a new ProjectRelease when this drifts from the
	// recomputed hash on a reconcile.
	// +kubebuilder:validation:MinLength=1
	Hash string `json:"hash"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=proj;projs

// Project is the Schema for the projects API.
type Project struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectSpec   `json:"spec,omitempty"`
	Status ProjectStatus `json:"status,omitempty"`
}

func (p *Project) GetConditions() []metav1.Condition {
	return p.Status.Conditions
}

func (p *Project) SetConditions(conditions []metav1.Condition) {
	p.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// ProjectList contains a list of Project.
type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Project `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Project{}, &ProjectList{})
}
