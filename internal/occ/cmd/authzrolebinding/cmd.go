// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrolebinding

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewAuthzRoleBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "authzrolebinding",
		Aliases: []string{"authzrolebindings", "arb"},
		Short:   "Manage authz role bindings",
		Long:    `Manage namespace-scoped authorization role bindings for OpenChoreo.`,
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
		Use:   "list",
		Short: "List authz role bindings",
		Long:  `List all authorization role bindings in a namespace.`,
		Example: `  # List all authz role bindings in a namespace
  occ authzrolebinding list --namespace acme-corp`,
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
		Use:   "get [AUTHZ_ROLE_BINDING_NAME]",
		Short: "Get an authz role binding",
		Long:  `Get an authorization role binding and display its details in YAML format.`,
		Example: `  # Get an authz role binding
  occ authzrolebinding get my-binding --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client.NewClient()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace: flags.GetNamespace(cmd),
				Name:      args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [AUTHZ_ROLE_BINDING_NAME]",
		Short: "Delete an authz role binding",
		Long:  `Delete an authorization role binding by name.`,
		Example: `  # Delete an authz role binding
  occ authzrolebinding delete my-binding --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client.NewClient()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace: flags.GetNamespace(cmd),
				Name:      args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
