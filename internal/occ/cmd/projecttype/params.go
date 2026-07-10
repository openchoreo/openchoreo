// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projecttype

// ListParams defines parameters for listing project types
type ListParams struct {
	Namespace string
}

// GetParams defines parameters for getting a single project type
type GetParams struct {
	Namespace       string
	ProjectTypeName string
}

// DeleteParams defines parameters for deleting a single project type
type DeleteParams struct {
	Namespace       string
	ProjectTypeName string
}
