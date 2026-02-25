// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

// ListParams defines parameters for listing workflow runs
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single workflow run
type GetParams struct {
	Namespace       string
	WorkflowRunName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }
