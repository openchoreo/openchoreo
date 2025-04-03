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

package project

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CreateProjImpl struct {
	config constants.CRDConfig
}

func NewCreateProjImpl(config constants.CRDConfig) *CreateProjImpl {
	return &CreateProjImpl{
		config: config,
	}
}

func (i *CreateProjImpl) CreateProject(params api.CreateProjectParams) error {
	if params.Interactive {
		return createProjectInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceProject, params); err != nil {
		return err
	}

	if err := validation.ValidateProjectName(params.Name); err != nil {
		return err
	}

	return createProject(params, i.config)
}

func createProject(params api.CreateProjectParams, config constants.CRDConfig) error {
	projRes, err := kinds.NewProjectResource(config, params.Organization)
	if err != nil {
		return fmt.Errorf("failed to create Project resource: %w", err)
	}

	if err := projRes.CreateProject(params); err != nil {
		return fmt.Errorf("failed to create project '%s' in organization '%s': %w",
			params.Name, params.Organization, err)
	}

	return nil
}
