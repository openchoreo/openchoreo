// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

// ListParams defines parameters for listing workflows
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// StartRunParams defines parameters for starting a workflow run
type StartRunParams struct {
	Namespace    string
	WorkflowName string
}
