// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

// ListParams defines parameters for listing project release bindings
type ListParams struct {
	Namespace string
	Project   string
}

// GetParams defines parameters for getting a single project release binding
type GetParams struct {
	Namespace                 string
	ProjectReleaseBindingName string
}

// DeleteParams defines parameters for deleting a single project release binding
type DeleteParams struct {
	Namespace                 string
	ProjectReleaseBindingName string
}
