// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

// ListParams defines parameters for listing components
type ListParams struct {
	Namespace string
	Project   string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// ScaffoldParams defines parameters for scaffolding a component
type ScaffoldParams struct {
	ComponentName string
	ComponentType string   // format: workloadType/componentTypeName
	Traits        []string // trait names
	WorkflowName  string
	Namespace     string
	ProjectName   string
	OutputPath    string
	SkipComments  bool // skip structural comments and field descriptions
	SkipOptional  bool // skip optional fields without defaults
}

// DeployParams defines parameters for deploying or promoting a component
type DeployParams struct {
	ComponentName string
	Namespace     string
	Project       string
	Release       string   // --release flag (optional release name)
	To            string   // --to flag (target env for promotion)
	Set           []string // --set values (type.path=value)
	OutputFormat  string
}

func (p DeployParams) GetNamespace() string     { return p.Namespace }
func (p DeployParams) GetProject() string       { return p.Project }
func (p DeployParams) GetComponentName() string { return p.ComponentName }

// GetParams defines parameters for getting a single component
type GetParams struct {
	Namespace     string
	ComponentName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single component
type DeleteParams struct {
	Namespace     string
	ComponentName string
}

func (p DeleteParams) GetNamespace() string     { return p.Namespace }
func (p DeleteParams) GetComponentName() string { return p.ComponentName }

// StartWorkflowParams defines parameters for starting a component's workflow
type StartWorkflowParams struct {
	Namespace     string
	ComponentName string
	Project       string
	Set           []string
}

func (p StartWorkflowParams) GetNamespace() string     { return p.Namespace }
func (p StartWorkflowParams) GetComponentName() string { return p.ComponentName }

// ListWorkflowRunsParams defines parameters for listing workflow runs by component
type ListWorkflowRunsParams struct {
	Namespace     string
	ComponentName string
}

func (p ListWorkflowRunsParams) GetNamespace() string     { return p.Namespace }
func (p ListWorkflowRunsParams) GetComponentName() string { return p.ComponentName }

// LogsParams defines parameters for fetching component logs
type LogsParams struct {
	Namespace   string
	Project     string
	Component   string
	Environment string
	Follow      bool
	Since       string // duration like "1h", "30m", "5m"
}
