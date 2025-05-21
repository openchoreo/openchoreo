/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package component

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type GetCompImpl struct {
	config constants.CRDConfig
}

func NewGetCompImpl(config constants.CRDConfig) *GetCompImpl {
	return &GetCompImpl{
		config: config,
	}
}

func (i *GetCompImpl) GetComponent(params api.GetComponentParams) error {
	if params.Interactive {
		return getComponentInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceComponent, params); err != nil {
		return err
	}

	return getComponents(params, i.config)
}

func getComponents(params api.GetComponentParams, config constants.CRDConfig) error {
	compRes, err := kinds.NewComponentResource(config, params.Organization, params.Project)
	if err != nil {
		return fmt.Errorf("failed to create Component resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return compRes.Print(format, filter)
}
