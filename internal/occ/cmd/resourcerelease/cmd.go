// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcerelease

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/specdiff"
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
		newDiffCmd(f),
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

func newDiffCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff [FROM_RESOURCE_RELEASE] [TO_RESOURCE_RELEASE]",
		Short: "Compare two resource releases",
		Long: `Compare the specs of two resource releases and show the fields that were added,
removed, or modified going from the first release to the second. Metadata
such as UIDs and timestamps is not compared.`,
		Example: `  # Compare two resource releases
  occ resourcerelease diff analytics-db-abc123 analytics-db-def456 --namespace acme-corp

  # Disable colored output
  occ resourcerelease diff analytics-db-abc123 analytics-db-def456 -n acme-corp --color never`,
		Args:    cmdutil.ExactArgsWithUsage(2),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := specdiff.GetColorMode(cmd)
			if err != nil {
				return err
			}
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Diff(DiffParams{
				Namespace: flags.GetNamespace(cmd),
				From:      args[0],
				To:        args[1],
				Color:     mode.Enabled(os.Stdout),
			})
		},
	}
	flags.AddNamespace(cmd)
	specdiff.AddColorFlag(cmd)
	return cmd
}
