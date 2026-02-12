// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EnvVar represents an environment variable present in the container.
type EnvVar struct {
	// The environment variable key.
	// +required
	Key string `json:"key"`

	// The literal value of the environment variable.
	// Mutually exclusive with valueFrom.
	// +optional
	Value string `json:"value,omitempty"`

	// Extract the environment variable value from another resource.
	// Mutually exclusive with value.
	// +optional
	ValueFrom *EnvVarValueFrom `json:"valueFrom,omitempty"`
}

// EnvVarValueFrom holds references to external sources for environment variables.
type EnvVarValueFrom struct {
	// Reference to a configuration group.
	// +optional
	ConfigurationGroupRef *ConfigurationGroupKeyRef `json:"configurationGroupRef,omitempty"`

	// Reference to a secret resource.
	// +optional
	SecretRef *SecretKeyRef `json:"secretRef,omitempty"`
}

// ConfigurationGroupKeyRef references a specific key in a configuration group.
type ConfigurationGroupKeyRef struct {
	// +required
	Name string `json:"name"`
	// +required
	Key string `json:"key"`
}

// SecretKeyRef references a specific key in a K8s secret.
type SecretKeyRef struct {
	// +required
	Name string `json:"name"`
	// +required
	Key string `json:"key"`
}

// FileVar represents a file configuration in a container.
// +kubebuilder:validation:XValidation:rule="has(self.value) != has(self.valueFrom)",message="exactly one of value or valueFrom must be set"
type FileVar struct {
	// The file key/name.
	// +kubebuilder:validation:Required
	Key string `json:"key"`

	// The mount path where the file will be mounted.
	// +kubebuilder:validation:Required
	MountPath string `json:"mountPath"`

	// The literal content of the file.
	// Mutually exclusive with valueFrom.
	// +optional
	Value string `json:"value,omitempty"`

	// Extract the environment variable value from another resource.
	// Mutually exclusive with value.
	// +optional
	ValueFrom *EnvVarValueFrom `json:"valueFrom,omitempty"`
}

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

	// File configurations.
	// +optional
	Files []FileVar `json:"files,omitempty"`
}

// EndpointType defines the different API technologies supported by the endpoint
type EndpointType string

const (
	EndpointTypeHTTP      EndpointType = "HTTP"
	EndpointTypeREST      EndpointType = "REST"
	EndpointTypeGraphQL   EndpointType = "GraphQL"
	EndpointTypeWebsocket EndpointType = "Websocket"
	EndpointTypeGRPC      EndpointType = "gRPC"
	EndpointTypeTCP       EndpointType = "TCP"
	EndpointTypeUDP       EndpointType = "UDP"
)

func (e EndpointType) String() string {
	return string(e)
}

// WorkloadEndpoint represents a simple network endpoint for basic exposure.
type WorkloadEndpoint struct {
	// Type indicates the protocol/technology of the endpoint (HTTP, REST, gRPC, GraphQL, Websocket, TCP, UDP).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=HTTP;REST;gRPC;GraphQL;Websocket;TCP;UDP
	Type EndpointType `json:"type"`

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
// +kubebuilder:validation:XValidation:rule="has(self.container) != has(self.containers)",message="exactly one of container or containers must be set"
type WorkloadTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Container defines the single container specification for this workload.
	// Mutually exclusive with containers.
	// +optional
	Container *Container `json:"container,omitempty"`

	// Containers define the container specifications for this workload.
	// The key is the container name, and the value is the container specification.
	// Mutually exclusive with container.
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
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="Component",type=string,JSONPath=`.spec.owner.componentName`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

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
