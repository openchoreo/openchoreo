// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewClusterObservabilityPlaneCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clusterobservabilityplane",
		Aliases: []string{"clusterobservabilityplanes", "cop"},
		Short:   "Manage cluster observability planes",
		Long:    `Manage cluster-scoped observability planes for OpenChoreo.`,
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
		Short: "List cluster observability planes",
		Long:  `List all cluster-scoped observability planes available across the cluster.`,
		Example: `  # List all cluster observability planes
  occ clusterobservabilityplane list`,
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
		Use:   "get [CLUSTER_OBSERVABILITY_PLANE_NAME]",
		Short: "Get a cluster observability plane",
		Long:  `Get a cluster observability plane and display its details in YAML format.`,
		Example: `  # Get a cluster observability plane
  occ clusterobservabilityplane get default`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{ClusterObservabilityPlaneName: args[0]})
		},
	}
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "delete [CLUSTER_OBSERVABILITY_PLANE_NAME]",
		Short: "Delete a cluster observability plane",
		Long:  `Delete a cluster observability plane by name.`,
		Example: `  # Delete a cluster observability plane
  occ clusterobservabilityplane delete default`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{ClusterObservabilityPlaneName: args[0]})
		},
	}
}
