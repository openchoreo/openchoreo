// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterauthzrole

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewClusterAuthzRoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clusterauthzrole",
		Aliases: []string{"clusterauthzroles", "car"},
		Short:   "Manage cluster authz roles",
		Long:    `Manage cluster-scoped authorization roles for OpenChoreo.`,
	}
	cmd.AddCommand(
		newListCmd(),
		newGetCmd(),
		newDeleteCmd(),
	)
	return cmd
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List cluster authz roles",
		Long:  `List all cluster-scoped authorization roles.`,
		Example: `  # List all cluster authz roles
  occ clusterauthzrole list`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client.NewClient()
			if err != nil {
				return err
			}
			return New(cl).List()
		},
	}
}

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [CLUSTER_AUTHZ_ROLE_NAME]",
		Short: "Get a cluster authz role",
		Long:  `Get a cluster authz role and display its details in YAML format.`,
		Example: `  # Get a cluster authz role
  occ clusterauthzrole get my-role`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client.NewClient()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{Name: args[0]})
		},
	}
}

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [CLUSTER_AUTHZ_ROLE_NAME]",
		Short: "Delete a cluster authz role",
		Long:  `Delete a cluster authz role by name.`,
		Example: `  # Delete a cluster authz role
  occ clusterauthzrole delete my-role`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client.NewClient()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{Name: args[0]})
		},
	}
}
