// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EndpointClassSpec defines the desired state of EndpointClass.
type EndpointClassSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	RESTPolicy *RESTEndpointPolicy `json:"restPolicy,omitempty"`
	GRPCPolicy *GRPCEndpointPolicy `json:"grpcPolicy,omitempty"`
}

// RESTEndpointPolicy defines REST-specific endpoint policies
type RESTEndpointPolicy struct {
	// Default policies that apply to all expose levels
	Defaults *RESTPolicyWithConditionals `json:"defaults,omitempty"`
	// Override policies for public expose level
	Public *RESTPolicyWithConditionals `json:"public,omitempty"`
	// Override policies for organization expose level
	Organization *RESTPolicyWithConditionals `json:"organization,omitempty"`
}

// GRPCEndpointPolicy defines gRPC-specific endpoint policies (placeholder for future implementation)
type GRPCEndpointPolicy struct {
	// TODO: Implement gRPC-specific policies
}

// RESTPolicy defines the base REST API management policies
type RESTPolicy struct {
	// Rate limiting configuration
	RateLimit *RateLimitPolicy `json:"rateLimit,omitempty"`
	// Authentication and authorization configuration
	Authentication *AuthenticationPolicy `json:"authentication,omitempty"`
	// CORS configuration
	CORS *CORSPolicy `json:"cors,omitempty"`
	// Security policies
	Security *SecurityPolicy `json:"security,omitempty"`
	// Request and response mediation/transformation
	Mediation *MediationPolicy `json:"mediation,omitempty"`
	// Request and response management
	Timeout           *metav1.Duration `json:"timeout,omitempty"`
	Retries           *RetryPolicy     `json:"retries,omitempty"`
	RequestSizeLimit  *string          `json:"requestSizeLimit,omitempty"`
	ResponseSizeLimit *string          `json:"responseSizeLimit,omitempty"`
	// Circuit breaker configuration
	CircuitBreaker *CircuitBreakerPolicy `json:"circuitBreaker,omitempty"`
	// Monitoring and logging configuration
	Monitoring *MonitoringPolicy `json:"monitoring,omitempty"`
}

// RESTConditionalPolicy defines a conditional policy that applies based on request conditions
type RESTConditionalPolicy struct {
	// Condition that must be met for this policy to apply
	Condition *RESTPolicyCondition `json:"condition,omitempty"`
	// Policy overrides to apply when condition matches
	Policy *RESTPolicy `json:"policy,omitempty"`
}

// RESTPolicyWithConditionals extends RESTPolicy to include conditional policies
type RESTPolicyWithConditionals struct {
	// Embed base REST policy
	RESTPolicy `json:",inline"`
	// Conditional policies that apply based on request conditions
	ConditionalPolicies []RESTConditionalPolicy `json:"conditionalPolicies,omitempty"`
}

// RESTPolicyCondition defines conditions for applying conditional policies
type RESTPolicyCondition struct {
	// HTTP method to match
	Method *string `json:"method,omitempty"`
	// Paths to match
	Paths []string `json:"paths,omitempty"`
}

// RateLimitPolicy defines rate limiting configuration
type RateLimitPolicy struct {
	Requests               int64   `json:"requests"`
	Window                 string  `json:"window"`
	Burst                  *int64  `json:"burst,omitempty"`
	KeyBy                  *string `json:"keyBy,omitempty"` // clientIP | header:X-API-Key | jwt:sub
	SkipSuccessfulRequests *bool   `json:"skipSuccessfulRequests,omitempty"`
	SkipFailedRequests     *bool   `json:"skipFailedRequests,omitempty"`
}

// AuthenticationPolicy defines authentication and authorization configuration
type AuthenticationPolicy struct {
	Type   string            `json:"type"` // jwt | apikey | oauth2 | basic
	JWT    *JWTAuthConfig    `json:"jwt,omitempty"`
	APIKey *APIKeyAuthConfig `json:"apikey,omitempty"`
	OAuth2 *OAuth2AuthConfig `json:"oauth2,omitempty"`
}

// JWTAuthConfig defines JWT authentication configuration
type JWTAuthConfig struct {
	JWKS     string   `json:"jwks"`
	Issuer   string   `json:"issuer"`
	Audience []string `json:"audience,omitempty"`
}

// APIKeyAuthConfig defines API key authentication configuration
type APIKeyAuthConfig struct {
	Header     *string `json:"header,omitempty"`
	QueryParam *string `json:"queryParam,omitempty"`
}

// OAuth2AuthConfig defines OAuth2 authentication configuration
type OAuth2AuthConfig struct {
	TokenURL string   `json:"tokenUrl"`
	Scopes   []string `json:"scopes,omitempty"`
}

// CORSPolicy defines CORS configuration
type CORSPolicy struct {
	AllowOrigins  []string `json:"allowOrigins,omitempty"`
	AllowMethods  []string `json:"allowMethods,omitempty"`
	AllowHeaders  []string `json:"allowHeaders,omitempty"`
	ExposeHeaders []string `json:"exposeHeaders,omitempty"`
	MaxAge        *int64   `json:"maxAge,omitempty"`
}

// SecurityPolicy defines security policies
type SecurityPolicy struct {
	AllowedIPs    []string `json:"allowedIPs,omitempty"`
	BlockedIPs    []string `json:"blockedIPs,omitempty"`
	RequireTLS    *bool    `json:"requireTLS,omitempty"`
	MinTLSVersion *string  `json:"minTLSVersion,omitempty"`
}

// MediationPolicy defines request and response transformation policies
type MediationPolicy struct {
	RequestTransformations  []TransformationRule `json:"requestTransformations,omitempty"`
	ResponseTransformations []TransformationRule `json:"responseTransformations,omitempty"`
}

// TransformationRule defines a single transformation rule
type TransformationRule struct {
	Type        string            `json:"type"`   // json | xml
	Action      string            `json:"action"` // addFields | addHeader | removeHeaders | removeFields
	Fields      map[string]string `json:"fields,omitempty"`
	Headers     []string          `json:"headers,omitempty"`
	HeaderName  *string           `json:"headerName,omitempty"`
	HeaderValue *string           `json:"headerValue,omitempty"`
}

// RetryPolicy defines retry configuration
type RetryPolicy struct {
	Attempts        int32  `json:"attempts"`
	Backoff         string `json:"backoff"` // exponential | linear | fixed
	InitialInterval string `json:"initialInterval"`
	MaxInterval     string `json:"maxInterval"`
}

// CircuitBreakerPolicy defines circuit breaker configuration
type CircuitBreakerPolicy struct {
	Enabled          bool   `json:"enabled"`
	ErrorThreshold   int32  `json:"errorThreshold"`
	SuccessThreshold int32  `json:"successThreshold"`
	Timeout          string `json:"timeout"`
}

// MonitoringPolicy defines monitoring and logging configuration
type MonitoringPolicy struct {
	Metrics *MetricsConfig `json:"metrics,omitempty"`
	Logging *LoggingConfig `json:"logging,omitempty"`
}

// MetricsConfig defines metrics configuration
type MetricsConfig struct {
	Enabled         bool  `json:"enabled"`
	DetailedMetrics *bool `json:"detailedMetrics,omitempty"`
}

// LoggingConfig defines logging configuration
type LoggingConfig struct {
	Enabled             bool   `json:"enabled"`
	LogLevel            string `json:"logLevel,omitempty"`
	IncludeRequestBody  *bool  `json:"includeRequestBody,omitempty"`
	IncludeResponseBody *bool  `json:"includeResponseBody,omitempty"`
}

// EndpointClassStatus defines the observed state of EndpointClass.
type EndpointClassStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EndpointClass is the Schema for the endpointclasses API.
type EndpointClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointClassSpec   `json:"spec,omitempty"`
	Status EndpointClassStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EndpointClassList contains a list of EndpointClass.
type EndpointClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EndpointClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EndpointClass{}, &EndpointClassList{})
}
