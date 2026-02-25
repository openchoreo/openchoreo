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
