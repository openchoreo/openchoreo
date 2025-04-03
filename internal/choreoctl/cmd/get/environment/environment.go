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

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type GetEnvironmentImpl struct {
	config constants.CRDConfig
}

func NewGetEnvironmentImpl(config constants.CRDConfig) *GetEnvironmentImpl {
	return &GetEnvironmentImpl{
		config: config,
	}
}

func (i *GetEnvironmentImpl) GetEnvironment(params api.GetEnvironmentParams) error {
	if params.Interactive {
		return getEnvironmentInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceEnvironment, params); err != nil {
		return err
	}

	return getEnvironments(params, i.config)
}

func getEnvironments(params api.GetEnvironmentParams, config constants.CRDConfig) error {
	envRes, err := kinds.NewEnvironmentResource(config, params.Organization)
	if err != nil {
		return fmt.Errorf("failed to create Environment resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return envRes.Print(format, filter)
}
