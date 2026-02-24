// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

// ListParams defines parameters for listing projects
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }
