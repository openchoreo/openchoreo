// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

// ListParams defines parameters for listing component types
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single component type
type GetParams struct {
	Namespace         string
	ComponentTypeName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single component type
type DeleteParams struct {
	Namespace         string
	ComponentTypeName string
}

func (p DeleteParams) GetNamespace() string         { return p.Namespace }
func (p DeleteParams) GetComponentTypeName() string { return p.ComponentTypeName }
