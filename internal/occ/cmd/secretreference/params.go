// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secretreference

// ListParams defines parameters for listing secret references
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single secret reference
type GetParams struct {
	Namespace           string
	SecretReferenceName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single secret reference
type DeleteParams struct {
	Namespace           string
	SecretReferenceName string
}

func (p DeleteParams) GetNamespace() string           { return p.Namespace }
func (p DeleteParams) GetSecretReferenceName() string { return p.SecretReferenceName }
