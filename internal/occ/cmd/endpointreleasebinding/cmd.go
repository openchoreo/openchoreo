// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewEndpointReleaseBindingCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "endpointreleasebinding",
		Aliases: []string{"endpointreleasebindings", "erb"},
		Short:   "Manage endpoint release bindings",
		Long:    "Commands for managing endpoint release bindings.",
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
		Short: "List endpoint release bindings",
		Long:  `List all endpoint release bindings in a namespace, optionally filtered by endpoint.`,
		Example: `  # List all endpoint release bindings in a namespace
  occ endpointreleasebinding list --namespace acme-corp

  # List bindings for a specific endpoint
  occ endpointreleasebinding list --namespace acme-corp --endpoint orders-api`,
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
		Use:   "get [ENDPOINT_RELEASE_BINDING_NAME]",
		Short: "Get an endpoint release binding",
		Long:  `Get an endpoint release binding and display its details in YAML format.`,
		Example: `  # Get an endpoint release binding
  occ endpointreleasebinding get orders-api-dev --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:                  flags.GetNamespace(cmd),
				EndpointReleaseBindingName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [ENDPOINT_RELEASE_BINDING_NAME]",
		Short: "Delete an endpoint release binding",
		Long:  `Delete an endpoint release binding by name.`,
		Example: `  # Delete an endpoint release binding
  occ endpointreleasebinding delete orders-api-dev --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:                  flags.GetNamespace(cmd),
				EndpointReleaseBindingName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
