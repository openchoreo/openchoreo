// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewProjectReleaseBindingCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "projectreleasebinding",
		Aliases: []string{"projectreleasebindings", "prb"},
		Short:   "Manage project release bindings",
		Long:    "Commands for managing project release bindings.",
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
		Short: "List project release bindings",
		Long:  `List all project release bindings in a namespace, optionally filtered by project.`,
		Example: `  # List all project release bindings in a namespace
  occ projectreleasebinding list --namespace acme-corp

  # List bindings for a specific project
  occ projectreleasebinding list --namespace acme-corp --project online-store`,
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
		Use:   "get [PROJECT_RELEASE_BINDING_NAME]",
		Short: "Get a project release binding",
		Long:  `Get a project release binding and display its details in YAML format.`,
		Example: `  # Get a project release binding
  occ projectreleasebinding get online-store-dev --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:                 flags.GetNamespace(cmd),
				ProjectReleaseBindingName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [PROJECT_RELEASE_BINDING_NAME]",
		Short: "Delete a project release binding",
		Long:  `Delete a project release binding by name.`,
		Example: `  # Delete a project release binding
  occ projectreleasebinding delete online-store-dev --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:                 flags.GetNamespace(cmd),
				ProjectReleaseBindingName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
