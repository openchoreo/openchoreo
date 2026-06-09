// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterresourcetype

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewClusterResourceTypeCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clusterresourcetype",
		Aliases: []string{"crt", "clusterresourcetypes"},
		Short:   "Manage cluster resource types",
		Long:    `Manage cluster-scoped resource types for OpenChoreo.`,
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
		Short: "List cluster resource types",
		Long:  `List all cluster-scoped resource types available across the cluster.`,
		Example: `  # List all cluster resource types
  occ clusterresourcetype list`,
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
		Use:   "get [CLUSTER_RESOURCE_TYPE_NAME]",
		Short: "Get a cluster resource type",
		Long:  `Get a cluster resource type and display its details in YAML format.`,
		Example: `  # Get a cluster resource type
  occ clusterresourcetype get mysql`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{ClusterResourceTypeName: args[0]})
		},
	}
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "delete [CLUSTER_RESOURCE_TYPE_NAME]",
		Short: "Delete a cluster resource type",
		Long:  `Delete a cluster resource type by name.`,
		Example: `  # Delete a cluster resource type
  occ clusterresourcetype delete mysql`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{ClusterResourceTypeName: args[0]})
		},
	}
}
