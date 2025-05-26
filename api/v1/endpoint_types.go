// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// EndpointDeletionFinalizer should be added as a finalizer to the
	// Endpoint whenever an endpoint is created. It should be cleared
	// During deletion when child external resources have been deleted
	EndpointDeletionFinalizer = "core.choreo.dev/endpoint-deletion"
)

// EndpointServiceSpec defines the configuration of the upstream service
type EndpointServiceSpec struct {
	// URL of the upstream service
	URL string `json:"url,omitempty"`

	// Base path of the upstream service
	// +optional
	BasePath string `json:"basePath,omitempty"`

	// Port of the upstream service
	// +required
	Port int32 `json:"port"`
}

// EndpointSchemaSpec defines the schema configuration of the endpoint
type EndpointSchemaSpec struct {
	// File path of the schema relative to the component source code
	FilePath string `json:"filePath,omitempty"`

	// Inline content of the schema
	Content string `json:"content,omitempty"`
}

// EndpointAPISettingsSpec defines configuration parameters for managed endpoints
type EndpointAPISettingsSpec struct {
	SecuritySchemes     []SecurityScheme  `json:"securitySchemes,omitempty"`
	AuthorizationHeader string            `json:"authorizationHeader,omitempty"`
	BackendJWT          *BackendJWTConfig `json:"backendJwt,omitempty"`
	OperationPolicies   []OperationPolicy `json:"operationPolicies,omitempty"`
	CORS                *CORSConfig       `json:"cors,omitempty"`
	RateLimit           *RateLimitConfig  `json:"rateLimit,omitempty"`
}

// BackendJWTConfig defines JWT configuration for backend services
type BackendJWTConfig struct {
	Enable        bool                    `json:"enable"`
	Configuration BackendJWTConfigDetails `json:"configuration"`
}

// BackendJWTConfigDetails contains the detailed JWT configuration
type BackendJWTConfigDetails struct {
	Audiences []string `json:"audiences"`
}

// OperationPolicy defines authentication policy for an API operation
type OperationPolicy struct {
	Target             string `json:"target"`
	AuthenticationType string `json:"authenticationType"`
}

// CORSConfig defines Cross-Origin Resource Sharing configuration
type CORSConfig struct {
	Enable        bool     `json:"enable"`
	AllowOrigins  []string `json:"allowOrigins"`
	AllowMethods  []string `json:"allowMethods"`
	AllowHeaders  []string `json:"allowHeaders"`
	ExposeHeaders []string `json:"exposeHeaders"`
}

// RateLimitConfig defines rate limiting configuration
type RateLimitConfig struct {
	Tier string `json:"tier"`
}

type SecurityScheme string

const (
	Oauth SecurityScheme = "oauth"
)

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

// EndpointSpec defines the desired state of Endpoint
type EndpointSpec struct {
	// Type indicates the protocol of the endpoint
	// +kubebuilder:validation:Enum=HTTP;REST;gRPC;GraphQL;Websocket;TCP;UDP
	Type EndpointType `json:"type"`

	// Configuration of the upstream service
	// +required
	Service EndpointServiceSpec `json:"service"`

	// Schema of the endpoint if available
	// +optional
	Schema *EndpointSchemaSpec `json:"schema,omitempty"`

	// Network visibility levels that the endpoint is exposed
	// +optional
	NetworkVisibilities *NetworkVisibility `json:"networkVisibilities,omitempty"`

	// Configuration parameters related to the managed endpoint
	// +optional
	APISettings *EndpointAPISettingsSpec `json:"apiSettings,omitempty"`
}

// NetworkVisibility defines the exposure configuration for different network levels of an Endpoint.
// It allows specifying visibility and security settings separately for organizational and public access.
// When configurations overlap with the Endpoint's APISettings, the most specific configuration takes precedence.
type NetworkVisibility struct {
	// When enabled, the endpoint is accessible to other services within the same organization.
	// +optional
	Organization *VisibilityConfig `json:"organization,omitempty"`

	// When enabled, the endpoint becomes accessible externally
	// +optional
	Public *VisibilityConfig `json:"public,omitempty"`
}

type VisibilityConfig struct {
	// +required
	Enable bool `json:"enable"`
	// +optional
	APISettings *EndpointAPISettingsSpec `json:"apiSettings,omitempty"`
}

// EndpointStatus defines the observed state of Endpoint
type EndpointStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	Address    string             `json:"address,omitempty"`
}

// Endpoint is the Schema for the endpoints API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Address",type="string",JSONPath=".status.address"
type Endpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointSpec   `json:"spec,omitempty"`
	Status EndpointStatus `json:"status,omitempty"`
}

func (ep *Endpoint) GetConditions() []metav1.Condition {
	return ep.Status.Conditions
}

func (ep *Endpoint) SetConditions(conditions []metav1.Condition) {
	ep.Status.Conditions = conditions
}

// EndpointList contains a list of Endpoint
// +kubebuilder:object:root=true
type EndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Endpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Endpoint{}, &EndpointList{})
}
