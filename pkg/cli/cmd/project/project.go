// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/project"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewProjectCmd() *cobra.Command {
	projectCmd := &cobra.Command{
		Use:     constants.Project.Use,
		Aliases: constants.Project.Aliases,
		Short:   constants.Project.Short,
		Long:    constants.Project.Long,
	}

	projectCmd.AddCommand(
		newListProjectCmd(),
		newGetProjectCmd(),
		newDeleteProjectCmd(),
	)

	return projectCmd
}

func newListProjectCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListProject,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			projectImpl := project.New()
			return projectImpl.List(project.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetProject.Use,
		Short:   constants.GetProject.Short,
		Long:    constants.GetProject.Long,
		Example: constants.GetProject.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)

			return project.New().Get(project.GetParams{
				Namespace:   namespace,
				ProjectName: args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}

func newDeleteProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteProject.Use,
		Short:   constants.DeleteProject.Short,
		Long:    constants.DeleteProject.Long,
		Example: constants.DeleteProject.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)

			return project.New().Delete(project.DeleteParams{
				Namespace:   namespace,
				ProjectName: args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}
