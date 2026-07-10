// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectrelease

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewProjectReleaseCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "projectrelease",
		Aliases: []string{"projectreleases"},
		Short:   "Manage project releases",
		Long:    "Commands for managing project releases.",
	}
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
	)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List project releases",
		Long:  `List all project releases in a namespace, optionally filtered by project.`,
		Example: `  # List all project releases in a namespace
  occ projectrelease list --namespace acme-corp

  # List releases for a specific project
  occ projectrelease list --namespace acme-corp --project online-store`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).List(ListParams{
				Namespace: flags.GetNamespace(cmd),
				Project:   flags.GetProject(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddProject(cmd)
	return cmd
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [PROJECT_RELEASE_NAME]",
		Short: "Get a project release",
		Long:  `Get a project release and display its details in YAML format.`,
		Example: `  # Get a project release
  occ projectrelease get online-store-abc123 --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:          flags.GetNamespace(cmd),
				ProjectReleaseName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [PROJECT_RELEASE_NAME]",
		Short: "Delete a project release",
		Long:  `Delete a project release by name.`,
		Example: `  # Delete a project release
  occ projectrelease delete online-store-abc123 --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:          flags.GetNamespace(cmd),
				ProjectReleaseName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
