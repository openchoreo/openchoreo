// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package deployment

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type GetDeploymentImpl struct {
	config constants.CRDConfig
}

func NewGetDeploymentImpl(config constants.CRDConfig) *GetDeploymentImpl {
	return &GetDeploymentImpl{
		config: config,
	}
}

func (i *GetDeploymentImpl) GetDeployment(params api.GetDeploymentParams) error {
	if params.Interactive {
		return getDeploymentInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceDeployment, params); err != nil {
		return err
	}

	return getDeployments(params, i.config)
}

func getDeployments(params api.GetDeploymentParams, config constants.CRDConfig) error {
	deployRes, err := kinds.NewDeploymentResource(
		config,
		params.Organization,
		params.Project,
		params.Component,
		params.Environment,
	)
	if err != nil {
		return fmt.Errorf("failed to create Deployment resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return deployRes.Print(format, filter)
}
