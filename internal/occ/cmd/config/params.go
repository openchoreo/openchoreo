// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

// AddContextParams defines parameters for adding a configuration context
type AddContextParams struct {
	Name         string
	ControlPlane string
	Credentials  string
	Namespace    string
	Project      string
	Component    string
}

// GetControlPlane returns the control plane name.
func (p AddContextParams) GetControlPlane() string { return p.ControlPlane }

// GetCredentials returns the credentials name.
func (p AddContextParams) GetCredentials() string { return p.Credentials }

// DeleteContextParams defines parameters for deleting a configuration context
type DeleteContextParams struct {
	Name string
}

// UpdateContextParams defines parameters for updating a configuration context
type UpdateContextParams struct {
	Name         string
	Namespace    string
	Project      string
	Component    string
	ControlPlane string
	Credentials  string
}

// UseContextParams defines parameters for switching the current context
type UseContextParams struct {
	Name string
}

// DescribeContextParams defines parameters for describing a context
type DescribeContextParams struct {
	Name string
}

// AddControlPlaneParams defines parameters for adding a control plane configuration
type AddControlPlaneParams struct {
	Name string
	URL  string
}

// UpdateControlPlaneParams defines parameters for updating a control plane configuration
type UpdateControlPlaneParams struct {
	Name string
	URL  string
}

// DeleteControlPlaneParams defines parameters for deleting a control plane configuration
type DeleteControlPlaneParams struct {
	Name string
}

// AddCredentialsParams defines parameters for adding credentials configuration
type AddCredentialsParams struct {
	Name string
}

// DeleteCredentialsParams defines parameters for deleting a credentials configuration
type DeleteCredentialsParams struct {
	Name string
}
