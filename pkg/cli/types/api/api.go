// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package api

// CommandImplementationInterface combines all APIs
type CommandImplementationInterface interface {
	ApplyAPI
	DeleteAPI
	LoginAPI
	LogoutAPI
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
