// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clustertrait"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewClusterTraitCmd() *cobra.Command {
	clusterTraitCmd := &cobra.Command{
		Use:     constants.ClusterTrait.Use,
		Aliases: constants.ClusterTrait.Aliases,
		Short:   constants.ClusterTrait.Short,
		Long:    constants.ClusterTrait.Long,
	}

	clusterTraitCmd.AddCommand(
		newListClusterTraitCmd(),
	)

	return clusterTraitCmd
}

func newListClusterTraitCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListClusterTrait,
		Flags:   []flags.Flag{},
		RunE: func(fg *builder.FlagGetter) error {
			return clustertrait.New().List()
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
