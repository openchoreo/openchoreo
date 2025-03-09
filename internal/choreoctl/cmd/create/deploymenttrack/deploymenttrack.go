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

package deploymenttrack

import (
	"fmt"

	"github.com/choreo-idp/choreo/internal/choreoctl/resources/kinds"
	"github.com/choreo-idp/choreo/internal/choreoctl/validation"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

type CreateDeploymentTrackImpl struct {
	config constants.CRDConfig
}

func NewCreateDeploymentTrackImpl(config constants.CRDConfig) *CreateDeploymentTrackImpl {
	return &CreateDeploymentTrackImpl{
		config: config,
	}
}

func (i *CreateDeploymentTrackImpl) CreateDeploymentTrack(params api.CreateDeploymentTrackParams) error {
	if params.Interactive {
		return createDeploymentTrackInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceDeploymentTrack, params); err != nil {
		return err
	}

	return createDeploymentTrack(params, i.config)
}

func createDeploymentTrack(params api.CreateDeploymentTrackParams, config constants.CRDConfig) error {
	trackRes, err := kinds.NewDeploymentTrackResource(config,
		params.Organization,
		params.Project,
		params.Component,
	)
	if err != nil {
		return fmt.Errorf("failed to create DeploymentTrack resource: %w", err)
	}

	if err := trackRes.CreateDeploymentTrack(params); err != nil {
		return fmt.Errorf("failed to create deployment track '%s' in component '%s' of project '%s' in organization '%s': %w",
			params.Name, params.Component, params.Project, params.Organization, err)
	}

	return nil
}
