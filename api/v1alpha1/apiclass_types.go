// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// APIClassSpec defines the desired state of APIClass.
type APIClassSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	RESTPolicy *RESTAPIPolicy `json:"restPolicy,omitempty"`
	GRPCPolicy *GRPCAPIPolicy `json:"grpcPolicy,omitempty"`
}

// RESTAPIPolicy defines REST-specific API policies
type RESTAPIPolicy struct {
	// Default policies that apply to all expose levels
	Defaults *RESTPolicy `json:"defaults,omitempty"`
	// Override policies for public expose level
	Public *RESTPolicy `json:"public,omitempty"`
	// Override policies for organization expose level
	Organization *RESTPolicy `json:"organization,omitempty"`
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
	// Circuit breaker configuration
	CircuitBreaker *CircuitBreakerPolicy `json:"circuitBreaker,omitempty"`
}

// RateLimitPolicy defines rate limiting configuration
type RateLimitPolicy struct {
	Requests int64  `json:"requests"` // Number of requests allowed
	Window   string `json:"window"`   // Time window (e.g., "1m", "1h", "30s", "1d")
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

// TransformationRule defines a single transformation rule
type TransformationRule struct {
	Type        string            `json:"type"`   // json | xml
	Action      string            `json:"action"` // addFields | addHeader | removeHeaders | removeFields
	Fields      map[string]string `json:"fields,omitempty"`
	Headers     []string          `json:"headers,omitempty"`
	HeaderName  *string           `json:"headerName,omitempty"`
	HeaderValue *string           `json:"headerValue,omitempty"`
}

// MediationPolicy defines request and response transformation policies
type MediationPolicy struct {
	RequestTransformations  []TransformationRule `json:"requestTransformations,omitempty"`
	ResponseTransformations []TransformationRule `json:"responseTransformations,omitempty"`
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
	Enabled bool `json:"enabled"`
	// +optional
	MaxConnections *int32 `json:"maxConnections"`
	// +optional
	MaxPendingRequests *int32 `json:"maxPendingRequests"`
	// +optional
	MaxParallelRequests *int32 `json:"maxParallelRequests"`
	// +optional
	MaxParallelRetries *int32 `json:"maxParallelRetries"`
}

// GRPCAPIPolicy defines gRPC-specific API policies (placeholder for future implementation)
type GRPCAPIPolicy struct {
	// TODO: Implement gRPC-specific policies
}

// APIClassStatus defines the observed state of APIClass.
type APIClassStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// APIClass is the Schema for the apiclasses API.
type APIClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIClassSpec   `json:"spec,omitempty"`
	Status APIClassStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// APIClassList contains a list of APIClass.
type APIClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIClass{}, &APIClassList{})
}
