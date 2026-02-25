// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

// CreateParams defines parameters for creating a workload from a descriptor
type CreateParams struct {
	FilePath      string
	NamespaceName string
	ProjectName   string
	ComponentName string
	ImageURL      string
	OutputPath    string
	DryRun        bool
	Mode          string // Operational mode: "api-server" or "file-system"
	RootDir       string // Root directory path for file-system mode
}

func (p CreateParams) GetNamespace() string { return p.NamespaceName }

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
