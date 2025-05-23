/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package deploymentpipeline

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// GetDeploymentPipelineImpl implements the GetDeploymentPipeline command
type GetDeploymentPipelineImpl struct {
	config constants.CRDConfig
}

// NewGetDeploymentPipelineImpl creates a new GetDeploymentPipelineImpl instance
func NewGetDeploymentPipelineImpl(config constants.CRDConfig) *GetDeploymentPipelineImpl {
	return &GetDeploymentPipelineImpl{
		config: config,
	}
}

// GetDeploymentPipeline gets deployment pipelines based on the provided parameters
func (i *GetDeploymentPipelineImpl) GetDeploymentPipeline(params api.GetDeploymentPipelineParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceDeploymentPipeline, params); err != nil {
		return err
	}

	return getDeploymentPipelines(params, i.config)
}

func getDeploymentPipelines(params api.GetDeploymentPipelineParams, config constants.CRDConfig) error {
	pipelineRes, err := kinds.NewDeploymentPipelineResource(config, params.Organization)
	if err != nil {
		return fmt.Errorf("failed to create DeploymentPipeline resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return pipelineRes.Print(format, filter)
}
