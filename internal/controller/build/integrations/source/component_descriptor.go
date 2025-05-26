// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package source

import choreov1 "github.com/openchoreo/openchoreo/api/v1"

// Config represents the root configuration structure
type Config struct {
	SchemaVersion string     `yaml:"schemaVersion"`
	Endpoints     []Endpoint `yaml:"endpoints"`
}

// Endpoint represents an individual service endpoint configuration
type Endpoint struct {
	Name                string                   `yaml:"name"`
	DisplayName         string                   `yaml:"displayName,omitempty"`
	Service             Service                  `yaml:"service"`
	NetworkVisibilities []NetworkVisibilityLevel `yaml:"networkVisibilities,omitempty"`
	Type                choreov1.EndpointType    `yaml:"type"`
}

// Service contains the service-specific configuration
type Service struct {
	BasePath string `yaml:"basePath,omitempty"`
	Port     int32  `yaml:"port"`
}

// NetworkVisibilityLevel defines the visibility level of an endpoint
type NetworkVisibilityLevel string

const (
	// NetworkVisibilityLevelProject indicates endpoint is only visible within the project
	NetworkVisibilityLevelProject NetworkVisibilityLevel = "Project"
	// NetworkVisibilityLevelOrganization indicates endpoint is visible within the organization
	NetworkVisibilityLevelOrganization NetworkVisibilityLevel = "Organization"
	// NetworkVisibilityLevelPublic indicates endpoint is publicly visible
	NetworkVisibilityLevelPublic NetworkVisibilityLevel = "Public"
)
