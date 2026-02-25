// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/dataplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewDataPlaneCmd() *cobra.Command {
	dataPlaneCmd := &cobra.Command{
		Use:     constants.DataPlane.Use,
		Aliases: constants.DataPlane.Aliases,
		Short:   constants.DataPlane.Short,
		Long:    constants.DataPlane.Long,
	}

	dataPlaneCmd.AddCommand(
		newListDataPlaneCmd(),
		newGetDataPlaneCmd(),
		newDeleteDataPlaneCmd(),
	)

	return dataPlaneCmd
}

func newListDataPlaneCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListDataPlane,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return dataplane.New().List(dataplane.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetDataPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetDataPlane.Use,
		Short:   constants.GetDataPlane.Short,
		Long:    constants.GetDataPlane.Long,
		Example: constants.GetDataPlane.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return dataplane.New().Get(dataplane.GetParams{
				Namespace:     namespace,
				DataPlaneName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}

func newDeleteDataPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteDataPlane.Use,
		Short:   constants.DeleteDataPlane.Short,
		Long:    constants.DeleteDataPlane.Long,
		Example: constants.DeleteDataPlane.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return dataplane.New().Delete(dataplane.DeleteParams{
				Namespace:     namespace,
				DataPlaneName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}
