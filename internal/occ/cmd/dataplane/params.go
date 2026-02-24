// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

// ListParams defines parameters for listing data planes
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }
