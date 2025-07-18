// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
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
	Protocol corev1.Protocol `json:"protocol"`

	// Port number for the endpoint.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Optional schema for the endpoint.
	// This can be used to define the actual API definition of the endpoint that is exposed by the workload.
	// +optional
	Schema *Schema `json:"schema,omitempty"`
}

// Schema defines the API definition for an endpoint.
type Schema struct {
	Type    string `json:"type,omitempty"`
	Content string `json:"content,omitempty"`
}

// WorkloadConnection represents an internal API connection
type WorkloadConnection struct {
	// Type of connection - only "api" for now
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=api
	Type string `json:"type"`

	// Parameters for connection configuration (dynamic key-value pairs)
	// +optional
	Params map[string]string `json:"params,omitempty"`

	// Inject defines how connection details are injected into the workload
	// +kubebuilder:validation:Required
	Inject WorkloadConnectionInject `json:"inject"`
}

// WorkloadConnectionInject defines how connection details are injected
type WorkloadConnectionInject struct {
	// Environment variables to inject
	// +kubebuilder:validation:Required
	Env []WorkloadConnectionEnvVar `json:"env"`
}

// WorkloadConnectionEnvVar defines an environment variable injection
type WorkloadConnectionEnvVar struct {
	// Environment variable name
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Template value using connection properties (e.g., "{{ .url }}")
	// +kubebuilder:validation:Required
	Value string `json:"value"`
}

// WorkloadTemplateSpec defines the desired state of Workload.
type WorkloadTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Containers define the container specifications for this workload.
	// The key is the container name, and the value is the container specification.
	// +optional
	Containers map[string]Container `json:"containers,omitempty"`

	// Endpoints define simple network endpoints for basic port exposure.
	// The key is the endpoint name, and the value is the endpoint specification.
	// +optional
	Endpoints map[string]WorkloadEndpoint `json:"endpoints,omitempty"`

	// Connections define how this workload consumes internal and external resources.
	// The key is the connection name, and the value is the connection specification.
	// +optional
	Connections map[string]WorkloadConnection `json:"connections,omitempty"`
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

// ConnectionTypeAPI represents an API connection type
const (
	ConnectionTypeAPI = "api"
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
