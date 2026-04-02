// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterdataplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	apiclient "github.com/openchoreo/openchoreo/pkg/cli/common/client"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewClusterDataPlaneCmd() *cobra.Command {
	clusterDataPlaneCmd := &cobra.Command{
		Use:     constants.ClusterDataPlane.Use,
		Aliases: constants.ClusterDataPlane.Aliases,
		Short:   constants.ClusterDataPlane.Short,
		Long:    constants.ClusterDataPlane.Long,
	}

	clusterDataPlaneCmd.AddCommand(
		newListClusterDataPlaneCmd(),
		newGetClusterDataPlaneCmd(),
		newDeleteClusterDataPlaneCmd(),
	)

	return clusterDataPlaneCmd
}

func newGetClusterDataPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetClusterDataPlane.Use,
		Short:   constants.GetClusterDataPlane.Short,
		Long:    constants.GetClusterDataPlane.Long,
		Example: constants.GetClusterDataPlane.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return clusterdataplane.New(cl).Get(clusterdataplane.GetParams{
				ClusterDataPlaneName: args[0],
			})
		},
	}
	return cmd
}

func newDeleteClusterDataPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteClusterDataPlane.Use,
		Short:   constants.DeleteClusterDataPlane.Short,
		Long:    constants.DeleteClusterDataPlane.Long,
		Example: constants.DeleteClusterDataPlane.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return clusterdataplane.New(cl).Delete(clusterdataplane.DeleteParams{
				ClusterDataPlaneName: args[0],
			})
		},
	}
	return cmd
}

func newListClusterDataPlaneCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListClusterDataPlane,
		Flags:   []flags.Flag{},
		RunE: func(fg *builder.FlagGetter) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return clusterdataplane.New(cl).List()
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
