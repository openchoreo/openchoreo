// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package organization

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type GetOrgImpl struct {
	config constants.CRDConfig
}

func NewGetOrgImpl(config constants.CRDConfig) *GetOrgImpl {
	return &GetOrgImpl{
		config: config,
	}
}

func (i *GetOrgImpl) GetOrganization(params api.GetParams) error {
	orgRes, err := kinds.NewOrganizationResource(i.config)
	if err != nil {
		return fmt.Errorf("failed to create Organization resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Name,
	}

	format := resources.OutputFormatTable
	if params.OutputFormat == constants.OutputFormatYAML {
		format = resources.OutputFormatYAML
	}

	return orgRes.Print(format, filter)
}
