// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebindingrevision

// ListParams defines parameters for listing a release binding's revisions
type ListParams struct {
	Namespace string
	// Binding lists one binding's revisions. Without it, the revisions of every
	// binding in the namespace are listed, narrowed by Project, Component and
	// Environment.
	Binding     string
	Project     string
	Component   string
	Environment string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting one release binding revision
type GetParams struct {
	Namespace string
	Binding   string
	Revision  int64
	Output    string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting one release binding revision
type DeleteParams struct {
	Namespace string
	Binding   string
	Revision  int64
	DryRun    bool
	Yes       bool
}

func (p DeleteParams) GetNamespace() string { return p.Namespace }
