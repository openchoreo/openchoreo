// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterauthzrole

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewClusterAuthzRoleCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clusterauthzrole",
		Aliases: []string{"clusterauthzroles", "car"},
		Short:   "Manage cluster authz roles",
		Long:    `Manage cluster-scoped authorization roles for OpenChoreo.`,
	}
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
	)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List cluster authz roles",
		Long:  `List all cluster-scoped authorization roles.`,
		Example: `  # List all cluster authz roles
  occ clusterauthzrole list`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).List()
		},
	}
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "get [CLUSTER_AUTHZ_ROLE_NAME]",
		Short: "Get a cluster authz role",
		Long:  `Get a cluster authz role and display its details in YAML format.`,
		Example: `  # Get a cluster authz role
  occ clusterauthzrole get my-role`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{Name: args[0]})
		},
	}
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "delete [CLUSTER_AUTHZ_ROLE_NAME]",
		Short: "Delete a cluster authz role",
		Long:  `Delete a cluster authz role by name.`,
		Example: `  # Delete a cluster authz role
  occ clusterauthzrole delete my-role`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{Name: args[0]})
		},
	}
}
