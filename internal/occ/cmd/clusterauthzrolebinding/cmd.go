// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterauthzrolebinding

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewClusterAuthzRoleBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clusterauthzrolebinding",
		Aliases: []string{"clusterauthzrolebindings", "carb"},
		Short:   "Manage cluster authz role bindings",
		Long:    `Manage cluster-scoped authorization role bindings for OpenChoreo.`,
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
		Short: "List cluster authz role bindings",
		Long:  `List all cluster-scoped authorization role bindings.`,
		Example: `  # List all cluster authz role bindings
  occ clusterauthzrolebinding list`,
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
		Use:   "get [CLUSTER_AUTHZ_ROLE_BINDING_NAME]",
		Short: "Get a cluster authz role binding",
		Long:  `Get a cluster authz role binding and display its details in YAML format.`,
		Example: `  # Get a cluster authz role binding
  occ clusterauthzrolebinding get my-binding`,
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
		Use:   "delete [CLUSTER_AUTHZ_ROLE_BINDING_NAME]",
		Short: "Delete a cluster authz role binding",
		Long:  `Delete a cluster authz role binding by name.`,
		Example: `  # Delete a cluster authz role binding
  occ clusterauthzrolebinding delete my-binding`,
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
