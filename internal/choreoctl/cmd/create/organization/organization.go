/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package organization

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CreateOrgImpl struct {
	config constants.CRDConfig
}

func NewCreateOrgImpl(config constants.CRDConfig) *CreateOrgImpl {
	return &CreateOrgImpl{
		config: config,
	}
}

func (i *CreateOrgImpl) CreateOrganization(params api.CreateOrganizationParams) error {
	if params.Interactive {
		return createOrganizationInteractive(i.config)
	}

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceOrganization, params); err != nil {
		return err
	}

	if err := validation.ValidateOrganizationName(params.Name); err != nil {
		return err
	}

	return createOrganization(params, i.config)
}

func createOrganization(params api.CreateOrganizationParams, config constants.CRDConfig) error {
	orgRes, err := kinds.NewOrganizationResource(config)
	if err != nil {
		return fmt.Errorf("failed to create Organization resource: %w", err)
	}

	if err := orgRes.CreateOrganization(params); err != nil {
		return fmt.Errorf("failed to create organization: %w", err)
	}

	return nil
}
