// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectrelease

// ListParams defines parameters for listing project releases
type ListParams struct {
	Namespace string
	Project   string
}

// GetParams defines parameters for getting a single project release
type GetParams struct {
	Namespace          string
	ProjectReleaseName string
}

// DeleteParams defines parameters for deleting a single project release
type DeleteParams struct {
	Namespace          string
	ProjectReleaseName string
}
