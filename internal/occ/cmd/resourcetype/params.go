// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcetype

// ListParams defines parameters for listing resource types
type ListParams struct {
	Namespace string
}

// GetParams defines parameters for getting a single resource type
type GetParams struct {
	Namespace        string
	ResourceTypeName string
}

// DeleteParams defines parameters for deleting a single resource type
type DeleteParams struct {
	Namespace        string
	ResourceTypeName string
}
