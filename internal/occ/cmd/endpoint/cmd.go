// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewEndpointCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "endpoint",
		Aliases: []string{"endpoints"},
		Short:   "Manage endpoints",
		Long:    `Manage endpoints for OpenChoreo.`,
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
		Short: "List endpoints",
		Long:  `List all endpoints in a namespace, optionally filtered by project.`,
		Example: `  # List all endpoints in a namespace
  occ endpoint list --namespace acme-corp

  # List endpoints in a specific project
  occ endpoint list --namespace acme-corp --project online-store`,
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
		Use:   "get [ENDPOINT_NAME]",
		Short: "Get an endpoint",
		Long:  `Get an endpoint and display its details in YAML format.`,
		Example: `  # Get an endpoint
  occ endpoint get orders-api --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:    flags.GetNamespace(cmd),
				EndpointName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [ENDPOINT_NAME]",
		Short: "Delete an endpoint",
		Long:  `Delete an endpoint by name.`,
		Example: `  # Delete an endpoint
  occ endpoint delete orders-api --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:    flags.GetNamespace(cmd),
				EndpointName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newPromoteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "promote [ENDPOINT_NAME]",
		Short: "Promote an endpoint to its latest release in a target environment",
		Long: `Promote advances the EndpointReleaseBinding for the target environment ` +
			`to the endpoint's latest release. The release name is read from ` +
			`Endpoint.status.latestRelease.`,
		Example: `  # Promote orders-api to its latest release in dev
  occ endpoint promote orders-api --env dev --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Promote(PromoteParams{
				Namespace:    flags.GetNamespace(cmd),
				EndpointName: args[0],
				Environment:  flags.GetEnvironment(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddEnvironment(cmd)
	return cmd
}
