// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CreateDataPlaneImpl struct {
	config constants.CRDConfig
}

func NewCreateDataPlaneImpl(config constants.CRDConfig) *CreateDataPlaneImpl {
	return &CreateDataPlaneImpl{
		config: config,
	}
}

func (i *CreateDataPlaneImpl) CreateDataPlane(params api.CreateDataPlaneParams) error {
	if params.Interactive {
		return createDataPlaneInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceDataPlane, params); err != nil {
		return err
	}

	return createDataPlane(params, i.config)
}

func createDataPlane(params api.CreateDataPlaneParams, config constants.CRDConfig) error {
	dpRes, err := kinds.NewDataPlaneResource(config, params.Organization)
	if err != nil {
		return fmt.Errorf("failed to create DataPlane resource: %w", err)
	}

	if err := dpRes.CreateDataPlane(params); err != nil {
		return fmt.Errorf("failed to create DataPlane '%s' in organization '%s': %w",
			params.Name, params.Organization, err)
	}

	return nil
}
