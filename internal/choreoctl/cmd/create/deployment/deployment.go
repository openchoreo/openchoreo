/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package deployment

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CreateDeploymentImpl struct {
	config constants.CRDConfig
}

func NewCreateDeploymentImpl(config constants.CRDConfig) *CreateDeploymentImpl {
	return &CreateDeploymentImpl{
		config: config,
	}
}

func (i *CreateDeploymentImpl) CreateDeployment(params api.CreateDeploymentParams) error {
	if params.Interactive {
		return createDeploymentInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceDeployment, params); err != nil {
		return err
	}

	return createDeployment(params, i.config)
}

func createDeployment(params api.CreateDeploymentParams, config constants.CRDConfig) error {
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

	if err := deployRes.CreateDeployment(params); err != nil {
		return fmt.Errorf("failed to create deployment '%s' in organization '%s': %w",
			params.Name, params.Organization, err)
	}

	return nil
}
