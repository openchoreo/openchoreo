// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcerelease

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewResourceReleaseCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resourcerelease",
		Aliases: []string{"resourcereleases"},
		Short:   "Manage resource releases",
		Long:    "Commands for managing resource releases.",
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
		Short: "List resource releases",
		Long:  `List all resource releases in a namespace, optionally filtered by resource.`,
		Example: `  # List all resource releases in a namespace
  occ resourcerelease list --namespace acme-corp

  # List releases for a specific resource
  occ resourcerelease list --namespace acme-corp --resource analytics-db`,
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
		Use:   "get [RESOURCE_RELEASE_NAME]",
		Short: "Get a resource release",
		Long:  `Get a resource release and display its details in YAML format.`,
		Example: `  # Get a resource release
  occ resourcerelease get analytics-db-abc123 --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:           flags.GetNamespace(cmd),
				ResourceReleaseName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [RESOURCE_RELEASE_NAME]",
		Short: "Delete a resource release",
		Long:  `Delete a resource release by name.`,
		Example: `  # Delete a resource release
  occ resourcerelease delete analytics-db-abc123 --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:           flags.GetNamespace(cmd),
				ResourceReleaseName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
