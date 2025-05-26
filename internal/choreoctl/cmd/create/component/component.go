// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CreateCompImpl struct {
	config constants.CRDConfig
}

func NewCreateCompImpl(config constants.CRDConfig) *CreateCompImpl {
	return &CreateCompImpl{
		config: config,
	}
}

func (i *CreateCompImpl) CreateComponent(params api.CreateComponentParams) error {
	if params.Interactive {
		return createComponentInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceComponent, params); err != nil {
		return err
	}

	return createComponent(params, i.config)
}

func createComponent(params api.CreateComponentParams, config constants.CRDConfig) error {
	compRes, err := kinds.NewComponentResource(config, params.Organization, params.Project)
	if err != nil {
		return fmt.Errorf("failed to create Component resource: %w", err)
	}

	if err := compRes.CreateComponent(params); err != nil {
		return fmt.Errorf("failed to create component '%s' in project '%s' of organization '%s': %w",
			params.Name, params.Project, params.Organization, err)
	}

	return nil
}
