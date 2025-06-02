// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deployableartifact

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type GetDeployableArtifactImpl struct {
	config constants.CRDConfig
}

func NewGetDeployableArtifactImpl(config constants.CRDConfig) *GetDeployableArtifactImpl {
	return &GetDeployableArtifactImpl{
		config: config,
	}
}

func (i *GetDeployableArtifactImpl) GetDeployableArtifact(params api.GetDeployableArtifactParams) error {
	if params.Interactive {
		return getDeployableArtifactInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceDeployableArtifact, params); err != nil {
		return err
	}

	return getDeployableArtifacts(params, i.config)
}

func getDeployableArtifacts(params api.GetDeployableArtifactParams, config constants.CRDConfig) error {
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

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return artifactRes.Print(format, filter)
}
