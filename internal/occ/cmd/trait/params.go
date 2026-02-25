// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

// ListParams defines parameters for listing traits
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single trait
type GetParams struct {
	Namespace string
	TraitName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single trait
type DeleteParams struct {
	Namespace string
	TraitName string
}

func (p DeleteParams) GetNamespace() string { return p.Namespace }
func (p DeleteParams) GetTraitName() string { return p.TraitName }
