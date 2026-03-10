// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowplane

// ListParams defines parameters for listing workflow planes
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single workflow plane
type GetParams struct {
	Namespace         string
	WorkflowPlaneName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single workflow plane
type DeleteParams struct {
	Namespace         string
	WorkflowPlaneName string
}

func (p DeleteParams) GetNamespace() string         { return p.Namespace }
func (p DeleteParams) GetWorkflowPlaneName() string { return p.WorkflowPlaneName }
