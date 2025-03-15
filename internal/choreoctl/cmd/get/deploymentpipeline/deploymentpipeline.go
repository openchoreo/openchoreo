/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package deploymentpipeline

import (
	"fmt"

	"github.com/choreo-idp/choreo/internal/choreoctl/resources"
	"github.com/choreo-idp/choreo/internal/choreoctl/resources/kinds"
	"github.com/choreo-idp/choreo/internal/choreoctl/validation"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
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
