// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewClusterTraitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clustertrait",
		Aliases: []string{"clustertraits"},
		Short:   "Manage cluster traits",
		Long:    `Manage cluster-scoped traits for OpenChoreo.`,
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
		Use:     "list",
		Short:   "List cluster traits",
		Long:    `List all cluster-scoped traits available across the cluster.`,
		Example: `  # List all cluster traits
  occ clustertrait list`,
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
		Use:     "get [CLUSTER_TRAIT_NAME]",
		Short:   "Get a cluster trait",
		Long:    `Get a cluster trait and display its details in YAML format.`,
		Example: `  # Get a cluster trait
  occ clustertrait get ingress`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client.NewClient()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{ClusterTraitName: args[0]})
		},
	}
}

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete [CLUSTER_TRAIT_NAME]",
		Short:   "Delete a cluster trait",
		Long:    `Delete a cluster trait by name.`,
		Example: `  # Delete a cluster trait
  occ clustertrait delete ingress`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client.NewClient()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{ClusterTraitName: args[0]})
		},
	}
}
