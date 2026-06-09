// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

// ListParams defines parameters for listing resources
type ListParams struct {
	Namespace string
	Project   string
}

// GetParams defines parameters for getting a single resource
type GetParams struct {
	Namespace    string
	ResourceName string
}

// DeleteParams defines parameters for deleting a single resource
type DeleteParams struct {
	Namespace    string
	ResourceName string
}

// PromoteParams defines parameters for promoting a resource to a target environment
type PromoteParams struct {
	Namespace    string
	ResourceName string
	Environment  string
}
