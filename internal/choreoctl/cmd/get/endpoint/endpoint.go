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

package endpoint

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type GetEndpointImpl struct {
	config constants.CRDConfig
}

func NewGetEndpointImpl(config constants.CRDConfig) *GetEndpointImpl {
	return &GetEndpointImpl{
		config: config,
	}
}

func (i *GetEndpointImpl) GetEndpoint(params api.GetEndpointParams) error {
	if params.Interactive {
		return getEndpointInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceEndpoint, params); err != nil {
		return err
	}

	return getEndpoints(params, i.config)
}

func getEndpoints(params api.GetEndpointParams, config constants.CRDConfig) error {
	endpointRes, err := kinds.NewEndpointResource(
		config,
		params.Organization,
		params.Project,
		params.Component,
		params.Environment,
	)
	if err != nil {
		return fmt.Errorf("failed to create Endpoint resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return endpointRes.Print(format, filter)
}
