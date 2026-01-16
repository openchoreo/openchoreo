// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package namespace

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type CreateNamespaceImpl struct {
	config constants.CRDConfig
}

func NewCreateNamespaceImpl(config constants.CRDConfig) *CreateNamespaceImpl {
	return &CreateNamespaceImpl{
		config: config,
	}
}

func (i *CreateNamespaceImpl) CreateNamespace(params api.CreateNamespaceParams) error {
	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceNamespace, params); err != nil {
		return err
	}

	if err := validation.ValidateNamespaceName(params.Name); err != nil {
		return err
	}

	return createNamespace(params, i.config)
}

func createNamespace(params api.CreateNamespaceParams, config constants.CRDConfig) error {
	namespaceRes, err := kinds.NewNamespaceResource(config)
	if err != nil {
		return fmt.Errorf("failed to create Namespace resource: %w", err)
	}

	if err := namespaceRes.CreateNamespace(params); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	return nil
}
