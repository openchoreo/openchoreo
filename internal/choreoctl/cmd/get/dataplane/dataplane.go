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

package dataplane

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type GetDataPlaneImpl struct {
	config constants.CRDConfig
}

func NewGetDataPlaneImpl(config constants.CRDConfig) *GetDataPlaneImpl {
	return &GetDataPlaneImpl{
		config: config,
	}
}

func (i *GetDataPlaneImpl) GetDataPlane(params api.GetDataPlaneParams) error {
	if params.Interactive {
		return getDataPlaneInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceDataPlane, params); err != nil {
		return err
	}

	return getDataPlanes(params, i.config)
}

func getDataPlanes(params api.GetDataPlaneParams, config constants.CRDConfig) error {
	dpRes, err := kinds.NewDataPlaneResource(config, params.Organization)
	if err != nil {
		return fmt.Errorf("failed to create DataPlane resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return dpRes.Print(format, filter)
}
