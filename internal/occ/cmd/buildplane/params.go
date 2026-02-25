// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

// ListParams defines parameters for listing build planes
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single build plane
type GetParams struct {
	Namespace      string
	BuildPlaneName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single build plane
type DeleteParams struct {
	Namespace      string
	BuildPlaneName string
}

func (p DeleteParams) GetNamespace() string      { return p.Namespace }
func (p DeleteParams) GetBuildPlaneName() string { return p.BuildPlaneName }
