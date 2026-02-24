// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

// ListParams defines parameters for listing build planes
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }
