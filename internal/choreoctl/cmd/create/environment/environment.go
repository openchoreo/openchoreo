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

package environment

import (
	"fmt"

	"github.com/choreo-idp/choreo/internal/choreoctl/resources/kinds"
	"github.com/choreo-idp/choreo/internal/choreoctl/validation"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

type CreateEnvironmentImpl struct {
	config constants.CRDConfig
}

func NewCreateEnvironmentImpl(config constants.CRDConfig) *CreateEnvironmentImpl {
	return &CreateEnvironmentImpl{
		config: config,
	}
}

func (i *CreateEnvironmentImpl) CreateEnvironment(params api.CreateEnvironmentParams) error {
	if params.Interactive {
		return createEnvironmentInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceEnvironment, params); err != nil {
		return err
	}

	return createEnvironment(params, i.config)
}

func createEnvironment(params api.CreateEnvironmentParams, config constants.CRDConfig) error {
	envRes, err := kinds.NewEnvironmentResource(config, params.Organization)
	if err != nil {
		return fmt.Errorf("failed to create Environment resource: %w", err)
	}

	if err := envRes.CreateEnvironment(params); err != nil {
		return fmt.Errorf("failed to create Environment '%s' in organization '%s': %w",
			params.Name, params.Organization, err)
	}

	return nil
}
