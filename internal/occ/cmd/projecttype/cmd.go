// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projecttype

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewProjectTypeCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "projecttype",
		Aliases: []string{"pt", "projecttypes"},
		Short:   "Manage project types",
		Long:    `Manage project types for OpenChoreo.`,
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
		Short: "List project types",
		Long:  `List all project types available in a namespace.`,
		Example: `  # List all project types in a namespace
  occ projecttype list --namespace acme-corp`,
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
		Use:   "get [PROJECT_TYPE_NAME]",
		Short: "Get a project type",
		Long:  `Get a project type and display its details in YAML format.`,
		Example: `  # Get a project type
  occ projecttype get web-service --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:       flags.GetNamespace(cmd),
				ProjectTypeName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [PROJECT_TYPE_NAME]",
		Short: "Delete a project type",
		Long:  `Delete a project type by name.`,
		Example: `  # Delete a project type
  occ projecttype delete web-service --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:       flags.GetNamespace(cmd),
				ProjectTypeName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
