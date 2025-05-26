// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package deploymenttrack

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type GetDeploymentTrackImpl struct {
	config constants.CRDConfig
}

func NewGetDeploymentTrackImpl(config constants.CRDConfig) *GetDeploymentTrackImpl {
	return &GetDeploymentTrackImpl{
		config: config,
	}
}

func (i *GetDeploymentTrackImpl) GetDeploymentTrack(params api.GetDeploymentTrackParams) error {
	if params.Interactive {
		return getDeploymentTrackInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceDeploymentTrack, params); err != nil {
		return err
	}

	return getDeploymentTracks(params, i.config)
}

func getDeploymentTracks(params api.GetDeploymentTrackParams, config constants.CRDConfig) error {
	trackRes, err := kinds.NewDeploymentTrackResource(
		config,
		params.Organization,
		params.Project,
		params.Component,
	)
	if err != nil {
		return fmt.Errorf("failed to create DeploymentTrack resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return trackRes.Print(format, filter)
}
