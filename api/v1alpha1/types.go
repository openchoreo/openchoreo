// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// This file contains common types shared across multiple OpenChoreo CRDs

// EndpointStatus represents the observed state of an endpoint
// Used by ServiceBinding, WebApplicationBinding, and other binding types
type EndpointStatus struct {
	// Name is the endpoint identifier matching spec.endpoints
	Name string `json:"name"`

	// Type is the endpoint type (uses EndpointType from endpoint_types.go)
	Type EndpointType `json:"type"`

	// Project contains access info for project-level visibility
	// +optional
	Project *EndpointAccess `json:"project,omitempty"`

	// Organization contains access info for organization-level visibility
	// +optional
	Organization *EndpointAccess `json:"organization,omitempty"`

	// Public contains access info for public visibility
	// +optional
	Public *EndpointAccess `json:"public,omitempty"`
}

// EndpointAccess contains all the information needed to connect to an endpoint
type EndpointAccess struct {
	// Host is the hostname or service name
	Host string `json:"host"`

	// Port is the port number
	Port int32 `json:"port"`

	// Scheme is the connection scheme (http, https, grpc, tcp)
	// +optional
	Scheme string `json:"scheme,omitempty"`

	// BasePath is the base URL path (for HTTP-based endpoints)
	// +optional
	BasePath string `json:"basePath,omitempty"`

	// URI is the computed URI for connecting to the endpoint
	// This field is automatically generated from host, port, scheme, and basePath
	// Examples: https://api.example.com:8080/v1, grpc://service:5050, tcp://localhost:9000
	// +optional
	URI string `json:"uri,omitempty"`

	// TODO: Add TLS and other details if needed
}

// EndpointExposeLevel defines the visibility scope for endpoint access
type EndpointExposeLevel string

const (
	// EndpointExposeLevelProject restricts endpoint access to components within the same project
	EndpointExposeLevelProject EndpointExposeLevel = "Project"

	// EndpointExposeLevelOrganization allows endpoint access across all projects within the same organization
	EndpointExposeLevelOrganization EndpointExposeLevel = "Organization"

	// EndpointExposeLevelPublic exposes the endpoint publicly, accessible from outside the organization
	EndpointExposeLevelPublic EndpointExposeLevel = "Public"
)
