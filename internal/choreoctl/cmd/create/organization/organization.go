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

package organization

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CreateOrgImpl struct {
	config constants.CRDConfig
}

func NewCreateOrgImpl(config constants.CRDConfig) *CreateOrgImpl {
	return &CreateOrgImpl{
		config: config,
	}
}

func (i *CreateOrgImpl) CreateOrganization(params api.CreateOrganizationParams) error {
	if params.Interactive {
		return createOrganizationInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceOrganization, params); err != nil {
		return err
	}

	if err := validation.ValidateOrganizationName(params.Name); err != nil {
		return err
	}

	return createOrganization(params, i.config)
}

func createOrganization(params api.CreateOrganizationParams, config constants.CRDConfig) error {
	orgRes, err := kinds.NewOrganizationResource(config)
	if err != nil {
		return fmt.Errorf("failed to create Organization resource: %w", err)
	}

	if err := orgRes.CreateOrganization(params); err != nil {
		return fmt.Errorf("failed to create organization: %w", err)
	}

	return nil
}
