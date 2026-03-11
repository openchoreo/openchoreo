// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterauthzrolebinding

// GetParams defines parameters for getting a single authz cluster role binding
type GetParams struct {
	Name string
}

// DeleteParams defines parameters for deleting a single authz cluster role binding
type DeleteParams struct {
	Name string
}
