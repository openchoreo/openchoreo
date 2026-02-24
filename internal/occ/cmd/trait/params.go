// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

// ListParams defines parameters for listing traits
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }
