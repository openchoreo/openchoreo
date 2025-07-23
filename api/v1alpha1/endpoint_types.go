// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ==============================================================================
// Constants and Finalizers
// ==============================================================================

const (
	// EndpointDeletionFinalizer should be added as a finalizer to the
	// Endpoint whenever an endpoint is created. It should be cleared
	// During deletion when child external resources have been deleted
	EndpointDeletionFinalizer = "openchoreo.dev/endpoint-deletion"
)

// ==============================================================================
// Backend Reference Types
// ==============================================================================

type BackendRefType string

const (
	// BackendRefTypeComponentRef indicates the backend reference is a component reference
	BackendRefTypeComponentRef BackendRefType = "componentRef"
	// BackendRefTypeTarget indicates the backend reference is a target URL
	BackendRefTypeTarget BackendRefType = "target"
)

// BackendRef defines the reference to the upstream service
type BackendRef struct {
	// Base path of the upstream service
	// +optional
	BasePath string `json:"basePath"`
	// type of the upstream service
	// +required
	// +kubebuilder:validation:Enum=componentRef;target
	Type         BackendRefType `json:"type"`
	ComponentRef *ComponentRef  `json:"componentRef,omitempty"`
	Target       *Target        `json:"target,omitempty"`
}

// ComponentRef defines the component reference for the upstream service
type ComponentRef struct {
	Port int32 `json:"port"`
}

// Target defines the target service URL for the upstream service. This is used for proxies
type Target struct {
	// URL of the upstream service
	URL string `json:"url"`
}

// ==============================================================================
// API Settings Configuration
// ==============================================================================

// EndpointAPISettingsSpec defines configuration parameters for managed endpoints
type EndpointAPISettingsSpec struct {
	AuthorizationHeader string            `json:"authorizationHeader,omitempty"`
	BackendJWT          *BackendJWTConfig `json:"backendJwt,omitempty"`
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

// ==============================================================================
// Owner and Template Types
// ==============================================================================

// EndpointOwner defines the owner reference for an endpoint
type EndpointOwner struct {
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// ==============================================================================
// Endpoint Types and Core Structures
// ==============================================================================

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

	// BackendRef is the reference to the backend service
	// +required
	BackendRef BackendRef `json:"backendRef"`

	// Network visibility levels that the endpoint is exposed
	// +optional
	NetworkVisibilities *NetworkVisibility `json:"networkVisibilities,omitempty"`
}

// ==============================================================================
// Network Visibility Configuration
// ==============================================================================

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
	Policies []Policy `json:"policies,omitempty"`
}

// ==============================================================================
// API-M Policy Configuration
// ==============================================================================

// PolicyType defines the type of API management policy
type PolicyType string

const (
	Oauth2PolicyType     PolicyType = "oauth2"
	APIKeyAuthPolicyType PolicyType = "api-key"
	BasicAuthPolicyType  PolicyType = "basic-auth"
	RateLimitPolicyType  PolicyType = "rate-limit"
	CORSPolicyType       PolicyType = "cors"
	MediationPolicyType  PolicyType = "mediation"
	// ToDo: Add more policy types as needed
)

// Policy defines an API management policy for an endpoint
type Policy struct {
	// +required
	Name string `json:"name"`
	// +required
	Type PolicyType `json:"type"`
	// +required
	*PolicySpec `json:"policySpec"`
}

// PolicySpec defines the configuration for different types of policies
type PolicySpec struct {
	// +optional
	APIKeyAuth *APIKeyAuthPolicySpec `json:"apiKeyAuth,omitempty"`
	// +optional
	BasicAuth *BasicAuthPolicySpec `json:"basicAuth,omitempty"`
	// +optional
	OAuth2 *OAuth2PolicySpec `json:"oauth2,omitempty"`
	// +optional
	RateLimit *RateLimitPolicySpec `json:"rateLimit,omitempty"`
	// +optional
	CORS *CORSPolicySpec `json:"cors,omitempty"`
	// +optional
	// MediationPolicies *[]MediationPolicy `json:"mediationPolicies,omitempty"`
	// ToDo: Add more policy types as needed
}

// ==============================================================================
// API-Key auth Policy Configuration
// ==============================================================================

type APIKeyAuthPolicySpec struct {
	KeySource  KeySourceDefinition `json:"keySource" yaml:"keySource"`
	SecretRefs []string            `json:"secretRefs" yaml:"secretRefs"`
}

type KeySourceDefinition struct {
	Header           string `json:"header" yaml:"header"`
	HeaderAuthScheme string `json:"headerAuthScheme,omitempty" yaml:"headerAuthScheme,omitempty"`
}

// ==============================================================================
// Basic auth Policy Configuration
// ==============================================================================

type BasicAuthPolicySpec struct {
	Users            []BasicAuthUser `json:"users" yaml:"users"`
	Header           string          `json:"header" yaml:"header"`
	HeaderAuthScheme string          `json:"headerAuthScheme" yaml:"headerAuthScheme"`
}

type BasicAuthUser struct {
	Username           string `json:"username" yaml:"username"`
	PasswordFromSecret string `json:"passwordFromSecret" yaml:"passwordFromSecret"`
}

// ==============================================================================
// OAuth2 Policy Configuration
// ==============================================================================

// OAuth2PolicySpec defines the configuration for OAuth2 policies
type OAuth2PolicySpec struct {
	// +required
	JWT JWT `json:"jwt" yaml:"jwt"`
}

type JWT struct {
	// +optional
	Claims *[]JWTClaim `json:"claims" yaml:"claims"`
	// +required
	Authorization AuthzSpec `json:"authorization" yaml:"authorization"`
}

type APIType string

const (
	APITypeREST    APIType = "REST"
	APITypeGRPC    APIType = "GRPC"
	APITypeGraphQL APIType = "GraphQL"
)

type AuthzSpec struct {
	// +required
	// +kubebuilder:validation:Enum=REST;GRPC;GraphQL
	APIType APIType `json:"apiType" yaml:"apiType"` // REST, GRPC, GraphQL
	// +optional
	Rest *REST `json:"rest" yaml:"rest"`
	// +optional
	GRPC *GRPC `json:"grpc" yaml:"grpc"`
	// +optional
	GraphQL *GraphQL `json:"graphql" yaml:"graphql"`
}

type JWTClaim struct {
	// +required
	Key string `json:"key" yaml:"key"`
	// +required
	Values []string `json:"values" yaml:"values"`
}

type ClaimToHeader struct {
	Name   string `json:"name" yaml:"name"`
	Header string `json:"header" yaml:"header"`
}

// ==============================================================================
// API Type-Specific Operations
// ==============================================================================

type REST struct {
	ClaimsToHeaders *[]ClaimToHeader `json:"claimsToHeaders" yaml:"claimsToHeaders"`
	Operations      *[]RESTOperation `json:"operations" yaml:"operations"`
}

type GRPC struct {
	ClaimsToHeaders *[]ClaimToHeader `json:"claimsToHeaders" yaml:"claimsToHeaders"`
	Operations      *[]GRPCOperation `json:"operations" yaml:"operations"`
}

type GraphQL struct {
	ClaimsToHeaders *[]ClaimToHeader    `json:"claimsToHeaders" yaml:"claimsToHeaders"`
	Operations      *[]GraphQLOperation `json:"operations" yaml:"operations"`
}

type RESTOperation struct {
	Target string     `json:"target" yaml:"target"`
	Method HTTPMethod `json:"method" yaml:"method"`
	Scopes []string   `json:"scopes" yaml:"scopes"`
}

// HTTPMethod describes how to select a HTTP route by matching the HTTP
// method as defined by
// [RFC 7231](https://datatracker.ietf.org/doc/html/rfc7231#section-4) and
// [RFC 5789](https://datatracker.ietf.org/doc/html/rfc5789#section-2).
// The value is expected in upper case.
//
// +kubebuilder:validation:Enum=GET;HEAD;POST;PUT;DELETE;CONNECT;OPTIONS;TRACE;PATCH
type HTTPMethod string

const (
	HTTPMethodGet     HTTPMethod = "GET"
	HTTPMethodHead    HTTPMethod = "HEAD"
	HTTPMethodPost    HTTPMethod = "POST"
	HTTPMethodPut     HTTPMethod = "PUT"
	HTTPMethodDelete  HTTPMethod = "DELETE"
	HTTPMethodConnect HTTPMethod = "CONNECT"
	HTTPMethodOptions HTTPMethod = "OPTIONS"
	HTTPMethodTrace   HTTPMethod = "TRACE"
	HTTPMethodPatch   HTTPMethod = "PATCH"
)

type GRPCOperation struct {
	Name    string              `json:"name" yaml:"name"`
	Methods []GRPCMethodDetails `json:"methods" yaml:"methods"`
}

type GRPCMethodDetails struct {
	Name   string   `json:"name" yaml:"name"`
	Scopes []string `json:"scopes" yaml:"scopes"`
}

type GraphQLOperation struct {
	Type   string   `json:"type" yaml:"type"` // query, mutation, subscription
	Name   string   `json:"name" yaml:"name"`
	Scopes []string `json:"scopes" yaml:"scopes"`
}

// ==============================================================================
// CORS Policies
// ==============================================================================

type CORSPolicySpec struct {
	AllowOrigins     []string `json:"allowOrigins" yaml:"allowOrigins"`
	AllowMethods     []string `json:"allowMethods" yaml:"allowMethods"`
	AllowHeaders     []string `json:"allowHeaders" yaml:"allowHeaders"`
	ExposeHeaders    []string `json:"exposeHeaders" yaml:"exposeHeaders"`
	MaxAge           int      `json:"maxAge" yaml:"maxAge"`
	AllowCredentials bool     `json:"allowCredentials" yaml:"allowCredentials"`
}

// ==============================================================================
// Rate Limiting Policies
// ==============================================================================

type RateLimitPolicySpec struct {
	APILevel       APILevelRLSpec       `json:"apiLevel" yaml:"apiLevel"`
	OperationLevel OperationLevelRLSpec `json:"operationLevel" yaml:"operationLevel"`
}

type APILevelRLSpec struct {
	TimeUnit     string `json:"timeUnit" yaml:"timeUnit"`
	RequestLimit int    `json:"requestLimit" yaml:"requestLimit"`
}

type OperationLevelRLSpec struct {
	REST *[]RestRLOperation `json:"rest" yaml:"rest"`
}

type RestRLOperation struct {
	Target       string `json:"target" yaml:"target"`
	Method       string `json:"method" yaml:"method"`
	TimeUnit     string `json:"timeUnit" yaml:"timeUnit"`
	RequestLimit int    `json:"requestLimit" yaml:"requestLimit"`
}

// ==============================================================================
// Mediation Policies Configuration
// ==============================================================================

// type MediationPolicy struct {
//	// ToDO: Finalize the mediation policy spec
// }

// ==============================================================================
// Endpoint Status
// ==============================================================================

// EndpointStatusLegacy defines the observed state of Endpoint
type EndpointStatusLegacy struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	Address    string             `json:"address,omitempty"`
}

// ==============================================================================
// Endpoint and EndpointList Resources
// ==============================================================================

// Endpoint is the Schema for the endpoints API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Address",type="string",JSONPath=".status.address"
type Endpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointSpec         `json:"spec,omitempty"`
	Status EndpointStatusLegacy `json:"status,omitempty"`
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
