// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointtype

// ListParams defines parameters for listing endpoint types
type ListParams struct {
	Namespace string
}

// GetParams defines parameters for getting a single endpoint type
type GetParams struct {
	Namespace        string
	EndpointTypeName string
}

// DeleteParams defines parameters for deleting a single endpoint type
type DeleteParams struct {
	Namespace        string
	EndpointTypeName string
}
