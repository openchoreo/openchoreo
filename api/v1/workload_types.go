// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WorkloadTemplateSpec defines the desired state of Workload.
type WorkloadTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The runtime “class” that provides defaults / templates.
	ClassName string `json:"className"`

	Type WorkloadType `json:"type"`

	// OCI image to run (digest or tag).
	Image string `json:"image"`

	// Container entrypoint & args.
	Command []string `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`

	// Explicit environment variables.
	// +optional
	Env []EnvVar `json:"env,omitempty"`

	// Bulk import environment variables from references.
	// +optional
	EnvFrom []EnvFromSource `json:"envFrom,omitempty"`

	// Single-file mounts.
	// +optional
	FileMounts []FileMount `json:"fileMounts,omitempty"`

	// Bulk import file mounts from references.
	// +optional
	FileMountsFrom []FileMountsFromSource `json:"fileMountsFrom,omitempty"`
}

type WorkloadOwner struct {
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

type WorkloadSpec struct {
	Owner WorkloadOwner `json:"owner"`

	// +kubebuilder:validation:MinLength=1
	EnvironmentName string `json:"environmentName"`
	// Inline *all* the template fields so they appear at top level.
	WorkloadTemplateSpec `json:",inline"`
}

// WorkloadType defines how the workload is deployed.
type WorkloadType string

const (
	WorkloadTypeService        WorkloadType = "Service"
	WorkloadTypeManualTask     WorkloadType = "ManualTask"
	WorkloadTypeScheduledTask  WorkloadType = "ScheduledTask"
	WorkloadTypeWebApplication WorkloadType = "WebApplication"
)

// WorkloadStatus defines the observed state of Workload.
type WorkloadStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Workload is the Schema for the workloads API.
type Workload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadSpec   `json:"spec,omitempty"`
	Status WorkloadStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkloadList contains a list of Workload.
type WorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workload `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workload{}, &WorkloadList{})
}
