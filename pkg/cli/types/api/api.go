// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package api

// CommandImplementationInterface combines all APIs
type CommandImplementationInterface interface {
	ApplyAPI
	DeleteAPI
	LoginAPI
	LogoutAPI
	ConfigContextAPI
	WorkloadAPI
	ComponentReleaseAPI
	ReleaseBindingAPI
}

// ApplyAPI defines methods for applying configurations
type ApplyAPI interface {
	Apply(params ApplyParams) error
}

// DeleteAPI defines methods for deleting resources from configuration files
type DeleteAPI interface {
	Delete(params DeleteParams) error
}

// LoginAPI defines methods for authentication
type LoginAPI interface {
	Login(params LoginParams) error
	IsLoggedIn() bool
	GetLoginPrompt() string
}

// LogoutAPI defines methods for ending sessions
type LogoutAPI interface {
	Logout() error
}

type ConfigContextAPI interface {
	AddContext(params AddContextParams) error
	ListContexts() error
	DeleteContext(params DeleteContextParams) error
	UpdateContext(params UpdateContextParams) error
	UseContext(params UseContextParams) error
	DescribeContext(params DescribeContextParams) error
	AddControlPlane(params AddControlPlaneParams) error
	ListControlPlanes() error
	UpdateControlPlane(params UpdateControlPlaneParams) error
	DeleteControlPlane(params DeleteControlPlaneParams) error
	AddCredentials(params AddCredentialsParams) error
	ListCredentials() error
	DeleteCredentials(params DeleteCredentialsParams) error
}

// WorkloadAPI defines methods for creating workloads from descriptors
type WorkloadAPI interface {
	CreateWorkload(params CreateWorkloadParams) error
}

// ComponentReleaseAPI defines component release operations (file-system mode)
type ComponentReleaseAPI interface {
	GenerateComponentRelease(params GenerateComponentReleaseParams) error
	ListComponentReleases(params ListComponentReleasesParams) error
}

// ReleaseBindingAPI defines release binding operations (file-system mode)
type ReleaseBindingAPI interface {
	GenerateReleaseBinding(params GenerateReleaseBindingParams) error
	ListReleaseBindings(params ListReleaseBindingsParams) error
}
