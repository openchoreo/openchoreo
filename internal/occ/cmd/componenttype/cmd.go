// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewComponentTypeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "componenttype",
		Aliases: []string{"ct", "componenttypes"},
		Short:   "Manage component types",
		Long:    `Manage component types for OpenChoreo.`,
	}
	cmd.AddCommand(
		newListCmd(),
		newGetCmd(),
		newDeleteCmd(),
	)
	return cmd
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List component types",
		Long:    `List all component types available in a namespace.`,
		Example: `  # List all component types in a namespace
  occ componenttype list --namespace acme-corp`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client.NewClient()
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

func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get [COMPONENT_TYPE_NAME]",
		Short:   "Get a component type",
		Long:    `Get a component type and display its details in YAML format.`,
		Example: `  # Get a component type
  occ componenttype get web-app --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client.NewClient()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:         flags.GetNamespace(cmd),
				ComponentTypeName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [COMPONENT_TYPE_NAME]",
		Short:   "Delete a component type",
		Long:    `Delete a component type by name.`,
		Example: `  # Delete a component type
  occ componenttype delete web-app --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client.NewClient()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:         flags.GetNamespace(cmd),
				ComponentTypeName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
