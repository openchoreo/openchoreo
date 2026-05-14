// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewResourceCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resource",
		Aliases: []string{"resources"},
		Short:   "Manage resources",
		Long:    `Manage resources for OpenChoreo.`,
	}
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
		newPromoteCmd(f),
	)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resources",
		Long:  `List all resources in a namespace, optionally filtered by project.`,
		Example: `  # List all resources in a namespace
  occ resource list --namespace acme-corp

  # List resources in a specific project
  occ resource list --namespace acme-corp --project online-store`,
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
		Use:   "get [RESOURCE_NAME]",
		Short: "Get a resource",
		Long:  `Get a resource and display its details in YAML format.`,
		Example: `  # Get a resource
  occ resource get analytics-db --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:    flags.GetNamespace(cmd),
				ResourceName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [RESOURCE_NAME]",
		Short: "Delete a resource",
		Long:  `Delete a resource by name.`,
		Example: `  # Delete a resource
  occ resource delete analytics-db --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:    flags.GetNamespace(cmd),
				ResourceName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newPromoteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "promote [RESOURCE_NAME]",
		Short: "Promote a resource to its latest release in a target environment",
		Long: `Promote advances the ResourceReleaseBinding for the target environment ` +
			`to the resource's latest release. The release name is read from ` +
			`Resource.status.latestRelease.`,
		Example: `  # Promote analytics-db to its latest release in dev
  occ resource promote analytics-db --env dev --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Promote(PromoteParams{
				Namespace:    flags.GetNamespace(cmd),
				ResourceName: args[0],
				Environment:  flags.GetEnvironment(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddEnvironment(cmd)
	return cmd
}
