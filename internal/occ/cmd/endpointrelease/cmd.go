// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrelease

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewEndpointReleaseCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "endpointrelease",
		Aliases: []string{"endpointreleases"},
		Short:   "Manage endpoint releases",
		Long:    "Commands for managing endpoint releases.",
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
		Short: "List endpoint releases",
		Long:  `List all endpoint releases in a namespace, optionally filtered by endpoint.`,
		Example: `  # List all endpoint releases in a namespace
  occ endpointrelease list --namespace acme-corp

  # List releases for a specific endpoint
  occ endpointrelease list --namespace acme-corp --endpoint orders-api`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).List(ListParams{
				Namespace: flags.GetNamespace(cmd),
				Endpoint:  flags.GetEndpoint(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddEndpoint(cmd)
	return cmd
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [ENDPOINT_RELEASE_NAME]",
		Short: "Get an endpoint release",
		Long:  `Get an endpoint release and display its details in YAML format.`,
		Example: `  # Get an endpoint release
  occ endpointrelease get orders-api-abc123 --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:           flags.GetNamespace(cmd),
				EndpointReleaseName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [ENDPOINT_RELEASE_NAME]",
		Short: "Delete an endpoint release",
		Long:  `Delete an endpoint release by name.`,
		Example: `  # Delete an endpoint release
  occ endpointrelease delete orders-api-abc123 --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:           flags.GetNamespace(cmd),
				EndpointReleaseName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
