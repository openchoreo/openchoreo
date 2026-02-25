// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/buildplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewBuildPlaneCmd() *cobra.Command {
	buildPlaneCmd := &cobra.Command{
		Use:     constants.BuildPlane.Use,
		Aliases: constants.BuildPlane.Aliases,
		Short:   constants.BuildPlane.Short,
		Long:    constants.BuildPlane.Long,
	}

	buildPlaneCmd.AddCommand(
		newListBuildPlaneCmd(),
		newGetBuildPlaneCmd(),
		newDeleteBuildPlaneCmd(),
	)

	return buildPlaneCmd
}

func newListBuildPlaneCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListBuildPlane,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return buildplane.New().List(buildplane.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetBuildPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetBuildPlane.Use,
		Short:   constants.GetBuildPlane.Short,
		Long:    constants.GetBuildPlane.Long,
		Example: constants.GetBuildPlane.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return buildplane.New().Get(buildplane.GetParams{
				Namespace:      namespace,
				BuildPlaneName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}

func newDeleteBuildPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteBuildPlane.Use,
		Short:   constants.DeleteBuildPlane.Short,
		Long:    constants.DeleteBuildPlane.Long,
		Example: constants.DeleteBuildPlane.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return buildplane.New().Delete(buildplane.DeleteParams{
				Namespace:      namespace,
				BuildPlaneName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}
