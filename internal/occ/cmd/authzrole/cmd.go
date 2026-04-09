// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrole

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewAuthzRoleCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "authzrole",
		Aliases: []string{"authzroles", "ar"},
		Short:   "Manage authz roles",
		Long:    `Manage namespace-scoped authorization roles for OpenChoreo.`,
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
		Short: "List authz roles",
		Long:  `List all authorization roles in a namespace.`,
		Example: `  # List all authz roles in a namespace
  occ authzrole list --namespace acme-corp`,
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
		Use:   "get [AUTHZ_ROLE_NAME]",
		Short: "Get an authz role",
		Long:  `Get an authorization role and display its details in YAML format.`,
		Example: `  # Get an authz role
  occ authzrole get my-role --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
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

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [AUTHZ_ROLE_NAME]",
		Short: "Delete an authz role",
		Long:  `Delete an authorization role by name.`,
		Example: `  # Delete an authz role
  occ authzrole delete my-role --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
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
