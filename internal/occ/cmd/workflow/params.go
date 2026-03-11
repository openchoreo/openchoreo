// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

// ListParams defines parameters for listing workflows
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single workflow
type GetParams struct {
	Namespace    string
	WorkflowName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// LogsParams defines parameters for getting workflow logs
type LogsParams struct {
	Namespace    string
	WorkflowName string
	RunName      string // optional --workflowrun flag; defaults to latest
	Follow       bool
	Since        string
}

func (p LogsParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single workflow
type DeleteParams struct {
	Namespace    string
	WorkflowName string
}

func (p DeleteParams) GetNamespace() string    { return p.Namespace }
func (p DeleteParams) GetWorkflowName() string { return p.WorkflowName }

// StartRunParams defines parameters for starting a workflow run
type StartRunParams struct {
	Namespace    string
	WorkflowName string
	RunName      string                 // optional; auto-generated if empty
	Parameters   map[string]interface{} // base parameters (e.g., from component workflow config)
	Set          []string               // --set overrides applied on top of Parameters
	Labels       map[string]string      // optional labels to attach to the workflow run
}
