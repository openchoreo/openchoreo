// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type GetEndpointImpl struct {
	config constants.CRDConfig
}

func NewGetEndpointImpl(config constants.CRDConfig) *GetEndpointImpl {
	return &GetEndpointImpl{
		config: config,
	}
}

func (i *GetEndpointImpl) GetEndpoint(params api.GetEndpointParams) error {
	if params.Interactive {
		return getEndpointInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceEndpoint, params); err != nil {
		return err
	}

	return getEndpoints(params, i.config)
}

func getEndpoints(params api.GetEndpointParams, config constants.CRDConfig) error {
	endpointRes, err := kinds.NewEndpointResource(
		config,
		params.Organization,
		params.Project,
		params.Component,
		params.Environment,
	)
	if err != nil {
		return fmt.Errorf("failed to create Endpoint resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return endpointRes.Print(format, filter)
}
