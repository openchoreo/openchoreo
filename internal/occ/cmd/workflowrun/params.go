// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

// ListParams defines parameters for listing workflow runs
type ListParams struct {
	Namespace string
	Workflow  string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single workflow run
type GetParams struct {
	Namespace       string
	WorkflowRunName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// LogsParams defines parameters for getting workflow run logs
type LogsParams struct {
	Namespace       string
	WorkflowRunName string
	Follow          bool
	Since           string
}

func (p LogsParams) GetNamespace() string { return p.Namespace }
