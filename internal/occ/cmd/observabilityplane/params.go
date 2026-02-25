// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

// ListParams defines parameters for listing observability planes
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single observability plane
type GetParams struct {
	Namespace              string
	ObservabilityPlaneName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single observability plane
type DeleteParams struct {
	Namespace              string
	ObservabilityPlaneName string
}

func (p DeleteParams) GetNamespace() string              { return p.Namespace }
func (p DeleteParams) GetObservabilityPlaneName() string { return p.ObservabilityPlaneName }
