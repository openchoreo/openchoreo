// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrole

// ListParams defines parameters for listing authz roles
type ListParams struct {
	Namespace string
}

// GetNamespace returns the namespace
func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single authz role
type GetParams struct {
	Namespace string
	Name      string
}

// GetNamespace returns the namespace
func (p GetParams) GetNamespace() string { return p.Namespace }
