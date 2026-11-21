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

// RollbackParams defines parameters for rolling a resource release binding back to one of
// its revisions
type RollbackParams struct {
	Namespace                  string
	ResourceReleaseBindingName string
	Revision                   int64
	DryRun                     bool
	Yes                        bool
	// Message is an optional reason for the rollback.
	Message string
}

func (p RollbackParams) GetNamespace() string { return p.Namespace }
