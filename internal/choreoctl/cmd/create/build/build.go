// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CreateBuildImpl struct {
	config constants.CRDConfig
}

func NewCreateBuildImpl(config constants.CRDConfig) *CreateBuildImpl {
	return &CreateBuildImpl{
		config: config,
	}
}

func (i *CreateBuildImpl) CreateBuild(params api.CreateBuildParams) error {
	if params.Interactive {
		return createBuildInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceBuild, params); err != nil {
		return err
	}

	return createBuild(params, i.config)
}

func createBuild(params api.CreateBuildParams, config constants.CRDConfig) error {
	buildRes, err := kinds.NewBuildResource(
		config,
		params.Organization,
		params.Project,
		params.Component,
		params.DeploymentTrack,
	)
	if err != nil {
		return fmt.Errorf("Failed to create Build resource: %w", err)
	}

	if err := buildRes.CreateBuild(params); err != nil {
		return fmt.Errorf("Failed to create build '%s' in organization '%s': %w",
			params.Name, params.Organization, err)
	}

	return nil
}
