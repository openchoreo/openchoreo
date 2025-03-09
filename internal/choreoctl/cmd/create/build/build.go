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

package build

import (
	"fmt"

	"github.com/choreo-idp/choreo/internal/choreoctl/resources/kinds"
	"github.com/choreo-idp/choreo/internal/choreoctl/validation"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

type CreateBuildImpl struct {
	config constants.CRDConfig
}

func NewCreateBuildImpl(config constants.CRDConfig) *CreateBuildImpl {
	return &CreateBuildImpl{
		config: config,
	}
}

func (i *CreateBuildImpl) CreateBuild(params api.CreateBuildParams) error {
	if params.Interactive {
		return createBuildInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceBuild, params); err != nil {
		return err
	}

	return createBuild(params, i.config)
}

func createBuild(params api.CreateBuildParams, config constants.CRDConfig) error {
	buildRes, err := kinds.NewBuildResource(
		config,
		params.Organization,
		params.Project,
		params.Component,
		params.DeploymentTrack,
	)
	if err != nil {
		return fmt.Errorf("Failed to create Build resource: %w", err)
	}

	if err := buildRes.CreateBuild(params); err != nil {
		return fmt.Errorf("Failed to create build '%s' in organization '%s': %w",
			params.Name, params.Organization, err)
	}

	return nil
}
