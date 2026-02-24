// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package api

// CommandImplementationInterface combines all APIs
type CommandImplementationInterface interface {
	ApplyAPI
	DeleteAPI
	LoginAPI
	LogoutAPI
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
