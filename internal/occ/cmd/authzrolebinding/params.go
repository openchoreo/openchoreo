// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrolebinding

// ListParams defines parameters for listing authz role bindings
type ListParams struct {
	Namespace string
}

// GetNamespace returns the namespace
func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single authz role binding
type GetParams struct {
	Namespace string
	Name      string
}

// GetNamespace returns the namespace
func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single authz role binding
type DeleteParams struct {
	Namespace string
	Name      string
}

// GetNamespace returns the namespace
func (p DeleteParams) GetNamespace() string { return p.Namespace }
