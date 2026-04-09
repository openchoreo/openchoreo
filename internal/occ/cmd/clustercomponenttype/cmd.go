// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewClusterComponentTypeCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clustercomponenttype",
		Aliases: []string{"cct", "clustercomponenttypes"},
		Short:   "Manage cluster component types",
		Long:    `Manage cluster-scoped component types for OpenChoreo.`,
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
		Short: "List cluster component types",
		Long:  `List all cluster-scoped component types available across the cluster.`,
		Example: `  # List all cluster component types
  occ clustercomponenttype list`,
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
		Use:   "get [CLUSTER_COMPONENT_TYPE_NAME]",
		Short: "Get a cluster component type",
		Long:  `Get a cluster component type and display its details in YAML format.`,
		Example: `  # Get a cluster component type
  occ clustercomponenttype get web-app`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{ClusterComponentTypeName: args[0]})
		},
	}
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "delete [CLUSTER_COMPONENT_TYPE_NAME]",
		Short: "Delete a cluster component type",
		Long:  `Delete a cluster component type by name.`,
		Example: `  # Delete a cluster component type
  occ clustercomponenttype delete web-app`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{ClusterComponentTypeName: args[0]})
		},
	}
}
