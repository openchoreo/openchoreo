// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

// ListParams defines parameters for listing resource release bindings
type ListParams struct {
	Namespace string
	Resource  string
}

// GetParams defines parameters for getting a single resource release binding
type GetParams struct {
	Namespace                  string
	ResourceReleaseBindingName string
}

// DeleteParams defines parameters for deleting a single resource release binding
type DeleteParams struct {
	Namespace                  string
	ResourceReleaseBindingName string
}
