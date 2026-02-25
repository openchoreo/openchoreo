// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

// ListParams defines parameters for listing workloads
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single workload
type GetParams struct {
	Namespace    string
	WorkloadName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single workload
type DeleteParams struct {
	Namespace    string
	WorkloadName string
}

func (p DeleteParams) GetNamespace() string    { return p.Namespace }
func (p DeleteParams) GetWorkloadName() string { return p.WorkloadName }
