// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflowplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewClusterWorkflowPlaneCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clusterworkflowplane",
		Aliases: []string{"clusterworkflowplanes", "cwp"},
		Short:   "Manage cluster workflow planes",
		Long:    `Manage cluster-scoped workflow planes for OpenChoreo.`,
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
		Short: "List cluster workflow planes",
		Long:  `List all cluster-scoped workflow planes available across the cluster.`,
		Example: `  # List all cluster workflow planes
  occ clusterworkflowplane list`,
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
		Use:   "get [CLUSTER_WORKFLOW_PLANE_NAME]",
		Short: "Get a cluster workflow plane",
		Long:  `Get a cluster workflow plane and display its details in YAML format.`,
		Example: `  # Get a cluster workflow plane
  occ clusterworkflowplane get default`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{ClusterWorkflowPlaneName: args[0]})
		},
	}
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "delete [CLUSTER_WORKFLOW_PLANE_NAME]",
		Short: "Delete a cluster workflow plane",
		Long:  `Delete a cluster workflow plane by name.`,
		Example: `  # Delete a cluster workflow plane
  occ clusterworkflowplane delete default`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{ClusterWorkflowPlaneName: args[0]})
		},
	}
}
