// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clustertrait"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	apiclient "github.com/openchoreo/openchoreo/pkg/cli/common/client"
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
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return clustertrait.New(cl).Get(clustertrait.GetParams{
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
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return clustertrait.New(cl).Delete(clustertrait.DeleteParams{
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
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return clustertrait.New(cl).List()
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
