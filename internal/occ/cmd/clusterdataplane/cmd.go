// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewClusterDataPlaneCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clusterdataplane",
		Aliases: []string{"clusterdataplanes", "cdp"},
		Short:   "Manage cluster data planes",
		Long:    `Manage cluster-scoped data planes for OpenChoreo.`,
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
		Short: "List cluster data planes",
		Long:  `List all cluster-scoped data planes available across the cluster.`,
		Example: `  # List all cluster data planes
  occ clusterdataplane list`,
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
		Use:   "get [CLUSTER_DATA_PLANE_NAME]",
		Short: "Get a cluster data plane",
		Long:  `Get a cluster data plane and display its details in YAML format.`,
		Example: `  # Get a cluster data plane
  occ clusterdataplane get default`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{ClusterDataPlaneName: args[0]})
		},
	}
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "delete [CLUSTER_DATA_PLANE_NAME]",
		Short: "Delete a cluster data plane",
		Long:  `Delete a cluster data plane by name.`,
		Example: `  # Delete a cluster data plane
  occ clusterdataplane delete default`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{ClusterDataPlaneName: args[0]})
		},
	}
}
