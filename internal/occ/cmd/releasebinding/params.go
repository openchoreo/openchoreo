// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

// GetParams defines parameters for getting a single release binding
type GetParams struct {
	Namespace          string
	ReleaseBindingName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single release binding
type DeleteParams struct {
	Namespace          string
	ReleaseBindingName string
}

func (p DeleteParams) GetNamespace() string          { return p.Namespace }
func (p DeleteParams) GetReleaseBindingName() string { return p.ReleaseBindingName }
