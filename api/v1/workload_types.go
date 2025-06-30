// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Container represents a single container in the workload.
type Container struct {
	// OCI image to run (digest or tag).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// Container entrypoint & args.
	// +optional
	Command []string `json:"command,omitempty"`
	// +optional
	Args []string `json:"args,omitempty"`

	// Explicit environment variables.
	// +optional
	Env []EnvVar `json:"env,omitempty"`
}

// WorkloadEndpoint represents a simple network endpoint for basic exposure.
type WorkloadEndpoint struct {
	// Network protocol (TCP, UDP, etc.).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Protocol string `json:"protocol"`

	// Port number for the endpoint.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
}

// WorkloadTemplateSpec defines the desired state of Workload.
type WorkloadTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The runtime “class” that provides defaults / templates.
	ClassName string `json:"className"` // DEPRECATED: The component-type-specific class will be used instead.

	Type WorkloadType `json:"type"` // DEPRECATED: Use the spec.type in the Component instead.

	// Containers define the container specifications for this workload.
	// The key is the container name, and the value is the container specification.
	// +optional
	Containers map[string]Container `json:"containers,omitempty"`

	// Endpoints define simple network endpoints for basic port exposure.
	// The key is the endpoint name, and the value is the endpoint specification.
	// +optional
	Endpoints map[string]WorkloadEndpoint `json:"endpoints,omitempty"`

	// Connections define how this workload connects to external resources.
	// This is a placeholder for future connection management capabilities.
	// +optional
	Connections map[string]string `json:"connections,omitempty"`
}

type WorkloadOwner struct {
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

type WorkloadSpec struct {
	Owner WorkloadOwner `json:"owner"`

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
