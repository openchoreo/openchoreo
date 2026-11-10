// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointtype

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewEndpointTypeCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "endpointtype",
		Aliases: []string{"et", "endpointtypes"},
		Short:   "Manage endpoint types",
		Long:    `Manage endpoint types for OpenChoreo.`,
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
		Short: "List endpoint types",
		Long:  `List all endpoint types available in a namespace.`,
		Example: `  # List all endpoint types in a namespace
  occ endpointtype list --namespace acme-corp`,
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
		Use:   "get [ENDPOINT_TYPE_NAME]",
		Short: "Get an endpoint type",
		Long:  `Get an endpoint type and display its details in YAML format.`,
		Example: `  # Get an endpoint type
  occ endpointtype get public-http --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:        flags.GetNamespace(cmd),
				EndpointTypeName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [ENDPOINT_TYPE_NAME]",
		Short: "Delete an endpoint type",
		Long:  `Delete an endpoint type by name.`,
		Example: `  # Delete an endpoint type
  occ endpointtype delete public-http --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:        flags.GetNamespace(cmd),
				EndpointTypeName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
