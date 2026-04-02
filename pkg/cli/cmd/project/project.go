// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/project"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	apiclient "github.com/openchoreo/openchoreo/pkg/cli/common/client"
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
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return project.New(cl).List(project.ListParams{
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
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return project.New(cl).Get(project.GetParams{
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
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return project.New(cl).Delete(project.DeleteParams{
				Namespace:   namespace,
				ProjectName: args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}
