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

	"github.com/choreo-idp/choreo/internal/choreoctl/resources"
	"github.com/choreo-idp/choreo/internal/choreoctl/resources/kinds"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

type GetOrgImpl struct {
	config constants.CRDConfig
}

func NewGetOrgImpl(config constants.CRDConfig) *GetOrgImpl {
	return &GetOrgImpl{
		config: config,
	}
}

func (i *GetOrgImpl) GetOrganization(params api.GetParams) error {
	orgRes, err := kinds.NewOrganizationResource(i.config)
	if err != nil {
		return fmt.Errorf("failed to create Organization resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return orgRes.Print(format, filter)
}
