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
		newGetClusterTraitCmd(),
		newDeleteClusterTraitCmd(),
	)

	return clusterTraitCmd
}

func newGetClusterTraitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetClusterTrait.Use,
		Short:   constants.GetClusterTrait.Short,
		Long:    constants.GetClusterTrait.Long,
		Example: constants.GetClusterTrait.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clustertrait.New().Get(clustertrait.GetParams{
				ClusterTraitName: args[0],
			})
		},
	}
	return cmd
}

func newDeleteClusterTraitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteClusterTrait.Use,
		Short:   constants.DeleteClusterTrait.Short,
		Long:    constants.DeleteClusterTrait.Long,
		Example: constants.DeleteClusterTrait.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clustertrait.New().Delete(clustertrait.DeleteParams{
				ClusterTraitName: args[0],
			})
		},
	}
	return cmd
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
