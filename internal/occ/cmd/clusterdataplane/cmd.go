// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewClusterDataPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clusterdataplane",
		Aliases: []string{"clusterdataplanes", "cdp"},
		Short:   "Manage cluster data planes",
		Long:    `Manage cluster-scoped data planes for OpenChoreo.`,
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
		Short:   "List cluster data planes",
		Long:    `List all cluster-scoped data planes available across the cluster.`,
		Example: `  # List all cluster data planes
  occ clusterdataplane list`,
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
		Use:     "get [CLUSTER_DATA_PLANE_NAME]",
		Short:   "Get a cluster data plane",
		Long:    `Get a cluster data plane and display its details in YAML format.`,
		Example: `  # Get a cluster data plane
  occ clusterdataplane get default`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client.NewClient()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{ClusterDataPlaneName: args[0]})
		},
	}
}

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete [CLUSTER_DATA_PLANE_NAME]",
		Short:   "Delete a cluster data plane",
		Long:    `Delete a cluster data plane by name.`,
		Example: `  # Delete a cluster data plane
  occ clusterdataplane delete default`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := client.NewClient()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{ClusterDataPlaneName: args[0]})
		},
	}
}
