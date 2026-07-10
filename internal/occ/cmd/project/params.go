// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

// ListParams defines parameters for listing projects
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single project
type GetParams struct {
	Namespace   string
	ProjectName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single project
type DeleteParams struct {
	Namespace   string
	ProjectName string
}

func (p DeleteParams) GetNamespace() string   { return p.Namespace }
func (p DeleteParams) GetProjectName() string { return p.ProjectName }

// DeployParams defines parameters for deploying or promoting a project
type DeployParams struct {
	Namespace   string
	ProjectName string
	To          string   // --to flag (target env for promotion)
	Release     string   // --release flag (optional explicit ProjectRelease name to pin)
	Set         []string // --set values (key=value) merged into spec.environmentConfigs
}

func (p DeployParams) GetNamespace() string   { return p.Namespace }
func (p DeployParams) GetProjectName() string { return p.ProjectName }

// ScaffoldParams defines parameters for scaffolding a Project from a (Cluster)ProjectType
type ScaffoldParams struct {
	ProjectName        string
	Namespace          string
	ProjectType        string // --projecttype (namespace-scoped)
	ClusterProjectType string // --clusterprojecttype (cluster-scoped)
	DeploymentPipeline string // --deployment-pipeline
	OutputPath         string // -o
	SkipComments       bool
	SkipOptional       bool
	NoBindings         bool // skip per-environment ProjectReleaseBinding output
}

func (p ScaffoldParams) GetNamespace() string   { return p.Namespace }
func (p ScaffoldParams) GetProjectName() string { return p.ProjectName }
