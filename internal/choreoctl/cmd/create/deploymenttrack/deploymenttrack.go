/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package deploymenttrack

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CreateDeploymentTrackImpl struct {
	config constants.CRDConfig
}

func NewCreateDeploymentTrackImpl(config constants.CRDConfig) *CreateDeploymentTrackImpl {
	return &CreateDeploymentTrackImpl{
		config: config,
	}
}

func (i *CreateDeploymentTrackImpl) CreateDeploymentTrack(params api.CreateDeploymentTrackParams) error {
	if params.Interactive {
		return createDeploymentTrackInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceDeploymentTrack, params); err != nil {
		return err
	}

	return createDeploymentTrack(params, i.config)
}

func createDeploymentTrack(params api.CreateDeploymentTrackParams, config constants.CRDConfig) error {
	trackRes, err := kinds.NewDeploymentTrackResource(config,
		params.Organization,
		params.Project,
		params.Component,
	)
	if err != nil {
		return fmt.Errorf("failed to create DeploymentTrack resource: %w", err)
	}

	if err := trackRes.CreateDeploymentTrack(params); err != nil {
		return fmt.Errorf("failed to create deployment track '%s' in component '%s' of project '%s' in organization '%s': %w",
			params.Name, params.Component, params.Project, params.Organization, err)
	}

	return nil
}
