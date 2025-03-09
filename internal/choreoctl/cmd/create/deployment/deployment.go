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

	"github.com/choreo-idp/choreo/internal/choreoctl/resources/kinds"
	"github.com/choreo-idp/choreo/internal/choreoctl/validation"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

type CreateDeploymentImpl struct {
	config constants.CRDConfig
}

func NewCreateDeploymentImpl(config constants.CRDConfig) *CreateDeploymentImpl {
	return &CreateDeploymentImpl{
		config: config,
	}
}

func (i *CreateDeploymentImpl) CreateDeployment(params api.CreateDeploymentParams) error {
	if params.Interactive {
		return createDeploymentInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceDeployment, params); err != nil {
		return err
	}

	return createDeployment(params, i.config)
}

func createDeployment(params api.CreateDeploymentParams, config constants.CRDConfig) error {
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

	if err := deployRes.CreateDeployment(params); err != nil {
		return fmt.Errorf("failed to create deployment '%s' in organization '%s': %w",
			params.Name, params.Organization, err)
	}

	return nil
}
