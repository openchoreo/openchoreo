// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcetype

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewResourceTypeCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resourcetype",
		Aliases: []string{"rt", "resourcetypes"},
		Short:   "Manage resource types",
		Long:    `Manage resource types for OpenChoreo.`,
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
		Short: "List resource types",
		Long:  `List all resource types available in a namespace.`,
		Example: `  # List all resource types in a namespace
  occ resourcetype list --namespace acme-corp`,
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
		Use:   "get [RESOURCE_TYPE_NAME]",
		Short: "Get a resource type",
		Long:  `Get a resource type and display its details in YAML format.`,
		Example: `  # Get a resource type
  occ resourcetype get mysql --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:        flags.GetNamespace(cmd),
				ResourceTypeName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [RESOURCE_TYPE_NAME]",
		Short: "Delete a resource type",
		Long:  `Delete a resource type by name.`,
		Example: `  # Delete a resource type
  occ resourcetype delete mysql --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:        flags.GetNamespace(cmd),
				ResourceTypeName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
