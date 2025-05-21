/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
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
