// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrelease

// ListParams defines parameters for listing endpoint releases
type ListParams struct {
	Namespace string
	Endpoint  string
}

// GetParams defines parameters for getting a single endpoint release
type GetParams struct {
	Namespace           string
	EndpointReleaseName string
}

// DeleteParams defines parameters for deleting a single endpoint release
type DeleteParams struct {
	Namespace           string
	EndpointReleaseName string
}
