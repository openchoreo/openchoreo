// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

// ListParams defines parameters for listing endpoint release bindings
type ListParams struct {
	Namespace string
	Endpoint  string
}

// GetParams defines parameters for getting a single endpoint release binding
type GetParams struct {
	Namespace                  string
	EndpointReleaseBindingName string
}

// DeleteParams defines parameters for deleting a single endpoint release binding
type DeleteParams struct {
	Namespace                  string
	EndpointReleaseBindingName string
}
