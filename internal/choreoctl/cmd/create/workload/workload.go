// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CreateWorkloadImpl struct {
	config constants.CRDConfig
}

func NewCreateWorkloadImpl(config constants.CRDConfig) *CreateWorkloadImpl {
	return &CreateWorkloadImpl{
		config: config,
	}
}

func (i *CreateWorkloadImpl) CreateWorkload(params api.CreateWorkloadParams) error {
	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceWorkload, params); err != nil {
		return err
	}

	return createWorkload(params, i.config)
}

func createWorkload(params api.CreateWorkloadParams, config constants.CRDConfig) error {
	workloadRes, err := kinds.NewWorkloadResource(config, params.OrganizationName)
	if err != nil {
		return fmt.Errorf("failed to create Workload resource: %w", err)
	}

	if err := workloadRes.CreateWorkload(params); err != nil {
		return fmt.Errorf("failed to create workload from descriptor '%s': %w", params.FilePath, err)
	}

	return nil
}
