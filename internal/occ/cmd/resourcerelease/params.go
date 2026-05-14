// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcerelease

// ListParams defines parameters for listing resource releases
type ListParams struct {
	Namespace string
	Resource  string
}

// GetParams defines parameters for getting a single resource release
type GetParams struct {
	Namespace           string
	ResourceReleaseName string
}

// DeleteParams defines parameters for deleting a single resource release
type DeleteParams struct {
	Namespace           string
	ResourceReleaseName string
}
