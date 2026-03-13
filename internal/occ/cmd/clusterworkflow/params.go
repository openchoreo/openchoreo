// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

// GetParams defines parameters for getting a single cluster workflow
type GetParams struct {
	ClusterWorkflowName string
}

// DeleteParams defines parameters for deleting a single cluster workflow
type DeleteParams struct {
	ClusterWorkflowName string
}

// StartRunParams defines parameters for starting a cluster workflow run
type StartRunParams struct {
	Namespace    string
	WorkflowName string
	Set          []string // --set overrides applied on top of Parameters
}

// LogsParams defines parameters for getting cluster workflow logs
type LogsParams struct {
	Namespace    string
	WorkflowName string
	RunName      string // optional --workflowrun flag; defaults to latest
	Follow       bool
	Since        string
}

func (p LogsParams) GetNamespace() string    { return p.Namespace }
func (p LogsParams) GetWorkflowName() string { return p.WorkflowName }
