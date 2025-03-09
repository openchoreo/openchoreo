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

	"github.com/choreo-idp/choreo/internal/choreoctl/resources/kinds"
	"github.com/choreo-idp/choreo/internal/choreoctl/validation"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

type CreateDataPlaneImpl struct {
	config constants.CRDConfig
}

func NewCreateDataPlaneImpl(config constants.CRDConfig) *CreateDataPlaneImpl {
	return &CreateDataPlaneImpl{
		config: config,
	}
}

func (i *CreateDataPlaneImpl) CreateDataPlane(params api.CreateDataPlaneParams) error {
	if params.Interactive {
		return createDataPlaneInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceDataPlane, params); err != nil {
		return err
	}

	return createDataPlane(params, i.config)
}

func createDataPlane(params api.CreateDataPlaneParams, config constants.CRDConfig) error {
	dpRes, err := kinds.NewDataPlaneResource(config, params.Organization)
	if err != nil {
		return fmt.Errorf("failed to create DataPlane resource: %w", err)
	}

	if err := dpRes.CreateDataPlane(params); err != nil {
		return fmt.Errorf("failed to create DataPlane '%s' in organization '%s': %w",
			params.Name, params.Organization, err)
	}

	return nil
}
