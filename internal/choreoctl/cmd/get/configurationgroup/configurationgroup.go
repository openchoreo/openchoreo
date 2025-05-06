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

package configurationgroup

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// GetConfigurationGroupImpl implements the GetConfigurationGroup command
type GetConfigurationGroupImpl struct {
	config constants.CRDConfig
}

// NewGetConfigurationGroupImpl creates a new GetConfigurationGroupImpl instance
func NewGetConfigurationGroupImpl(config constants.CRDConfig) *GetConfigurationGroupImpl {
	return &GetConfigurationGroupImpl{
		config: config,
	}
}

// GetConfigurationGroup gets the configuration groups based on the provided parameters
func (i *GetConfigurationGroupImpl) GetConfigurationGroup(params api.GetConfigurationGroupParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceConfigurationGroup, params); err != nil {
		return err
	}

	return getConfigurationGroups(params, i.config)
}

func getConfigurationGroups(params api.GetConfigurationGroupParams, config constants.CRDConfig) error {
	configGroupRes, err := kinds.NewConfigurationGroupResource(config, params.Organization)
	if err != nil {
		return fmt.Errorf("failed to retrieve ConfigurationGroup resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return configGroupRes.Print(format, filter)
}
