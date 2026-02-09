// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

// StoredConfig is the structure to store configuration contexts.
type StoredConfig struct {
	CurrentContext string         `yaml:"currentContext"`
	ControlPlanes  []ControlPlane `yaml:"controlplanes"`
	Credentials    []Credential   `yaml:"credentials,omitempty"`
	Contexts       []Context      `yaml:"contexts"`
}

// ControlPlane defines OpenChoreo API server configuration
type ControlPlane struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// Credential represents authentication credentials
type Credential struct {
	Name         string `yaml:"name"`
	ClientID     string `yaml:"clientId,omitempty"`
	ClientSecret string `yaml:"clientSecret,omitempty"`
	Token        string `yaml:"token,omitempty"`
	RefreshToken string `yaml:"refreshToken,omitempty"`
	AuthMethod   string `yaml:"authMethod,omitempty"` // "pkce" or "client_credentials"
}

// Context represents a single named configuration context.
type Context struct {
	Name              string `yaml:"name"`
	ControlPlane      string `yaml:"controlplane"`          // Reference to controlplanes[].name
	Credentials       string `yaml:"credentials,omitempty"` // Reference to credentials[].name
	Namespace         string `yaml:"namespace,omitempty"`
	Project           string `yaml:"project,omitempty"`
	Component         string `yaml:"component,omitempty"`
	Mode              string `yaml:"mode,omitempty"`              // "api-server" or "file-system"
	RootDirectoryPath string `yaml:"rootDirectoryPath,omitempty"` // Path for file-system mode
}

const (
	ModeAPIServer  = "api-server"
	ModeFileSystem = "file-system"
)
