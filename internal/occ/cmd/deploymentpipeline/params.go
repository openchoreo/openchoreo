// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

// ListParams defines parameters for listing deployment pipelines
type ListParams struct {
	Namespace string
}

func (p ListParams) GetNamespace() string { return p.Namespace }

// GetParams defines parameters for getting a single deployment pipeline
type GetParams struct {
	Namespace              string
	DeploymentPipelineName string
}

func (p GetParams) GetNamespace() string { return p.Namespace }

// DeleteParams defines parameters for deleting a single deployment pipeline
type DeleteParams struct {
	Namespace              string
	DeploymentPipelineName string
}

func (p DeleteParams) GetNamespace() string              { return p.Namespace }
func (p DeleteParams) GetDeploymentPipelineName() string { return p.DeploymentPipelineName }
