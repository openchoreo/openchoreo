// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package deployableartifact

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CreateDeployableArtifactImpl struct {
	config constants.CRDConfig
}

func NewCreateDeployableArtifactImpl(config constants.CRDConfig) *CreateDeployableArtifactImpl {
	return &CreateDeployableArtifactImpl{
		config: config,
	}
}

func (i *CreateDeployableArtifactImpl) CreateDeployableArtifact(params api.CreateDeployableArtifactParams) error {
	if params.Interactive {
		return createDeployableArtifactInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceDeployableArtifact, params); err != nil {
		return err
	}

	return createDeployableArtifact(params, i.config)
}

func createDeployableArtifact(params api.CreateDeployableArtifactParams, config constants.CRDConfig) error {
	artifactRes, err := kinds.NewDeployableArtifactResource(
		config,
		params.Organization,
		params.Project,
		params.Component,
		params.DeploymentTrack,
	)
	if err != nil {
		return fmt.Errorf("failed to create DeployableArtifact resource: %w", err)
	}

	if err := artifactRes.CreateDeployableArtifact(params); err != nil {
		return fmt.Errorf("failed to create deployable artifact '%s' in organization '%s': %w",
			params.Name, params.Organization, err)
	}

	return nil
}
