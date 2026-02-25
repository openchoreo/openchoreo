// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

// GenerateParams defines parameters for generating component releases
type GenerateParams struct {
	All           bool   // Generate for all components
	ProjectName   string // Generate for all components in this project
	ComponentName string // Generate for specific component
	ReleaseName   string // Optional: custom release name (only valid with --component)
	OutputPath    string // Optional: custom output directory
	DryRun        bool   // Preview without writing files
	Mode          string // Operational mode: "api-server" or "file-system"
	RootDir       string // Root directory path for file-system mode
}

// ListParams defines parameters for listing component releases
type ListParams struct {
	Namespace string
	Project   string
	Component string
}

func (p ListParams) GetNamespace() string { return p.Namespace }
func (p ListParams) GetProject() string   { return p.Project }
func (p ListParams) GetComponent() string { return p.Component }

// GetParams defines parameters for getting a single component release
type GetParams struct {
	Namespace            string
	ComponentReleaseName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }
