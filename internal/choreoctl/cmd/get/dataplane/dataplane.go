// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type GetDataPlaneImpl struct {
	config constants.CRDConfig
}

func NewGetDataPlaneImpl(config constants.CRDConfig) *GetDataPlaneImpl {
	return &GetDataPlaneImpl{
		config: config,
	}
}

func (i *GetDataPlaneImpl) GetDataPlane(params api.GetDataPlaneParams) error {
	if params.Interactive {
		return getDataPlaneInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceDataPlane, params); err != nil {
		return err
	}

	return getDataPlanes(params, i.config)
}

func getDataPlanes(params api.GetDataPlaneParams, config constants.CRDConfig) error {
	dpRes, err := kinds.NewDataPlaneResource(config, params.Organization)
	if err != nil {
		return fmt.Errorf("failed to create DataPlane resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return dpRes.Print(format, filter)
}
