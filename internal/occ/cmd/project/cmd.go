// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewProjectCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Aliases: []string{"proj", "projects"},
		Short:   "Manage projects",
		Long:    `Manage projects for OpenChoreo.`,
	}
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
		newDeployCmd(f),
	)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		Long:  `List all projects in a namespace.`,
		Example: `  # List all projects in a namespace
  occ project list --namespace acme-corp`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).List(ListParams{
				Namespace: flags.GetNamespace(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [PROJECT_NAME]",
		Short: "Get a project",
		Long:  `Get a project and display its details in YAML format.`,
		Example: `  # Get a project
  occ project get my-project --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:   flags.GetNamespace(cmd),
				ProjectName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [PROJECT_NAME]",
		Short: "Delete a project",
		Long:  `Delete a project by name.`,
		Example: `  # Delete a project
  occ project delete my-project --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:   flags.GetNamespace(cmd),
				ProjectName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeployCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [PROJECT_NAME]",
		Short: "Deploy or promote a project",
		Long: "Deploy a project's ProjectReleaseBinding to the lowest environment in " +
			"the pipeline, or promote it to the next environment.",
		Example: `  # Deploy to the lowest environment (controller seeds the latest release)
  occ project deploy online-store --namespace acme-corp

  # Promote to a specific environment
  occ project deploy online-store --namespace acme-corp --to staging

  # Pin an explicit release
  occ project deploy online-store --namespace acme-corp --release online-store-abc123`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Deploy(DeployParams{
				Namespace:   flags.GetNamespace(cmd),
				ProjectName: args[0],
				To:          flags.GetTo(cmd),
				Release:     flags.GetRelease(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddTo(cmd)
	flags.AddRelease(cmd)
	return cmd
}
