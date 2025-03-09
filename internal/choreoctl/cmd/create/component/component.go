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

package component

import (
	"fmt"

	"github.com/choreo-idp/choreo/internal/choreoctl/resources/kinds"
	"github.com/choreo-idp/choreo/internal/choreoctl/validation"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

type CreateCompImpl struct {
	config constants.CRDConfig
}

func NewCreateCompImpl(config constants.CRDConfig) *CreateCompImpl {
	return &CreateCompImpl{
		config: config,
	}
}

func (i *CreateCompImpl) CreateComponent(params api.CreateComponentParams) error {
	if params.Interactive {
		return createComponentInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceComponent, params); err != nil {
		return err
	}

	return createComponent(params, i.config)
}

func createComponent(params api.CreateComponentParams, config constants.CRDConfig) error {
	compRes, err := kinds.NewComponentResource(config, params.Organization, params.Project)
	if err != nil {
		return fmt.Errorf("failed to create Component resource: %w", err)
	}

	if err := compRes.CreateComponent(params); err != nil {
		return fmt.Errorf("failed to create component '%s' in project '%s' of organization '%s': %w",
			params.Name, params.Project, params.Organization, err)
	}

	return nil
}
