// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

// GetParams defines parameters for getting a single component release
type GetParams struct {
	Namespace            string
	ComponentReleaseName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }
