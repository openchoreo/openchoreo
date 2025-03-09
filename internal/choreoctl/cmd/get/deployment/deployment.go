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

package deployment

import (
	"fmt"

	"github.com/choreo-idp/choreo/internal/choreoctl/resources"
	"github.com/choreo-idp/choreo/internal/choreoctl/resources/kinds"
	"github.com/choreo-idp/choreo/internal/choreoctl/validation"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

type GetDeploymentImpl struct {
	config constants.CRDConfig
}

func NewGetDeploymentImpl(config constants.CRDConfig) *GetDeploymentImpl {
	return &GetDeploymentImpl{
		config: config,
	}
}

func (i *GetDeploymentImpl) GetDeployment(params api.GetDeploymentParams) error {
	if params.Interactive {
		return getDeploymentInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceDeployment, params); err != nil {
		return err
	}

	return getDeployments(params, i.config)
}

func getDeployments(params api.GetDeploymentParams, config constants.CRDConfig) error {
	deployRes, err := kinds.NewDeploymentResource(
		config,
		params.Organization,
		params.Project,
		params.Component,
		params.Environment,
	)
	if err != nil {
		return fmt.Errorf("failed to create Deployment resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return deployRes.Print(format, filter)
}
