// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

// GenerateParams defines parameters for generating release bindings
type GenerateParams struct {
	All              bool   // Generate for all components
	ProjectName      string // Generate for all components in this project
	ComponentName    string // Generate for specific component
	ComponentRelease string // Explicit component release name (only with project+component)
	TargetEnv        string // Required: target environment name
	UsePipeline      string // Required: deployment pipeline name
	OutputPath       string // Optional: custom output directory
	DryRun           bool   // Preview without writing files
	Mode             string // Operational mode: "api-server" or "file-system"
	RootDir          string // Root directory path for file-system mode
}

// ListParams defines parameters for listing release bindings
type ListParams struct {
	Namespace string
	Project   string
	Component string
}

func (p ListParams) GetNamespace() string { return p.Namespace }
func (p ListParams) GetProject() string   { return p.Project }
func (p ListParams) GetComponent() string { return p.Component }

// GetParams defines parameters for getting a single release binding
type GetParams struct {
	Namespace          string
	ReleaseBindingName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single release binding
type DeleteParams struct {
	Namespace          string
	ReleaseBindingName string
}

func (p DeleteParams) GetNamespace() string          { return p.Namespace }
func (p DeleteParams) GetReleaseBindingName() string { return p.ReleaseBindingName }
