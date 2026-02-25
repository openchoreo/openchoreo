// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/componenttype"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewComponentTypeCmd() *cobra.Command {
	componentTypeCmd := &cobra.Command{
		Use:     constants.ComponentType.Use,
		Aliases: constants.ComponentType.Aliases,
		Short:   constants.ComponentType.Short,
		Long:    constants.ComponentType.Long,
	}

	componentTypeCmd.AddCommand(
		newListComponentTypeCmd(),
		newGetComponentTypeCmd(),
		newDeleteComponentTypeCmd(),
	)

	return componentTypeCmd
}

func newGetComponentTypeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetComponentType.Use,
		Short:   constants.GetComponentType.Short,
		Long:    constants.GetComponentType.Long,
		Example: constants.GetComponentType.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)

			return componenttype.New().Get(componenttype.GetParams{
				Namespace:         namespace,
				ComponentTypeName: args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}

func newDeleteComponentTypeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteComponentType.Use,
		Short:   constants.DeleteComponentType.Short,
		Long:    constants.DeleteComponentType.Long,
		Example: constants.DeleteComponentType.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)

			return componenttype.New().Delete(componenttype.DeleteParams{
				Namespace:         namespace,
				ComponentTypeName: args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}

func newListComponentTypeCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListComponentType,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return componenttype.New().List(componenttype.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
