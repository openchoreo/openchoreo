// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewResourceReleaseBindingCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resourcereleasebinding",
		Aliases: []string{"resourcereleasebindings", "rrb"},
		Short:   "Manage resource release bindings",
		Long:    "Commands for managing resource release bindings.",
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
		Short: "List resource release bindings",
		Long:  `List all resource release bindings in a namespace, optionally filtered by resource.`,
		Example: `  # List all resource release bindings in a namespace
  occ resourcereleasebinding list --namespace acme-corp

  # List bindings for a specific resource
  occ resourcereleasebinding list --namespace acme-corp --resource analytics-db`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).List(ListParams{
				Namespace: flags.GetNamespace(cmd),
				Resource:  flags.GetResource(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddResource(cmd)
	return cmd
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [RESOURCE_RELEASE_BINDING_NAME]",
		Short: "Get a resource release binding",
		Long:  `Get a resource release binding and display its details in YAML format.`,
		Example: `  # Get a resource release binding
  occ resourcereleasebinding get analytics-db-dev --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:                  flags.GetNamespace(cmd),
				ResourceReleaseBindingName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [RESOURCE_RELEASE_BINDING_NAME]",
		Short: "Delete a resource release binding",
		Long:  `Delete a resource release binding by name.`,
		Example: `  # Delete a resource release binding
  occ resourcereleasebinding delete analytics-db-dev --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:                  flags.GetNamespace(cmd),
				ResourceReleaseBindingName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
