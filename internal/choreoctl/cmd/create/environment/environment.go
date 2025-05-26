// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CreateEnvironmentImpl struct {
	config constants.CRDConfig
}

func NewCreateEnvironmentImpl(config constants.CRDConfig) *CreateEnvironmentImpl {
	return &CreateEnvironmentImpl{
		config: config,
	}
}

func (i *CreateEnvironmentImpl) CreateEnvironment(params api.CreateEnvironmentParams) error {
	if params.Interactive {
		return createEnvironmentInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceEnvironment, params); err != nil {
		return err
	}

	return createEnvironment(params, i.config)
}

func createEnvironment(params api.CreateEnvironmentParams, config constants.CRDConfig) error {
	envRes, err := kinds.NewEnvironmentResource(config, params.Organization)
	if err != nil {
		return fmt.Errorf("failed to create Environment resource: %w", err)
	}

	if err := envRes.CreateEnvironment(params); err != nil {
		return fmt.Errorf("failed to create Environment '%s' in organization '%s': %w",
			params.Name, params.Organization, err)
	}

	return nil
}
