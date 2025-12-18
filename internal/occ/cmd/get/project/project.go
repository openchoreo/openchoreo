// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/resources"
	"github.com/openchoreo/openchoreo/internal/occ/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type GetProjImpl struct {
	config constants.CRDConfig
}

func NewGetProjImpl(config constants.CRDConfig) *GetProjImpl {
	return &GetProjImpl{
		config: config,
	}
}

func (i *GetProjImpl) GetProject(params api.GetProjectParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceProject, params); err != nil {
		return err
	}

	return getProjects(params, i.config)
}

func getProjects(params api.GetProjectParams, config constants.CRDConfig) error {
	projRes, err := kinds.NewProjectResource(config, params.Organization)
	if err != nil {
		return fmt.Errorf("failed to create Project resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name:  params.Name,
		Limit: params.Limit,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return projRes.Print(format, filter)
}
