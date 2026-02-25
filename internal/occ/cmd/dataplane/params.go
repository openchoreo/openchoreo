// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

// ListParams defines parameters for listing data planes
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single data plane
type GetParams struct {
	Namespace     string
	DataPlaneName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single data plane
type DeleteParams struct {
	Namespace     string
	DataPlaneName string
}

func (p DeleteParams) GetNamespace() string     { return p.Namespace }
func (p DeleteParams) GetDataPlaneName() string { return p.DataPlaneName }
