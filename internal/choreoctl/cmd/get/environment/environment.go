/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
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
