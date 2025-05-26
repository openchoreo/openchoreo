// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type GetBuildImpl struct {
	config constants.CRDConfig
}

func NewGetBuildImpl(config constants.CRDConfig) *GetBuildImpl {
	return &GetBuildImpl{
		config: config,
	}
}

func (i *GetBuildImpl) GetBuild(params api.GetBuildParams) error {
	if params.Interactive {
		return getBuildInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceBuild, params); err != nil {
		return err
	}

	return getBuilds(params, i.config)
}

func getBuilds(params api.GetBuildParams, config constants.CRDConfig) error {
	buildRes, err := kinds.NewBuildResource(
		config,
		params.Organization,
		params.Project,
		params.Component,
		params.DeploymentTrack,
	)
	if err != nil {
		return fmt.Errorf("failed to create Build resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return buildRes.Print(format, filter)
}
