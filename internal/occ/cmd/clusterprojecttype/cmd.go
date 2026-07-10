// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterprojecttype

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewClusterProjectTypeCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clusterprojecttype",
		Aliases: []string{"cpt", "clusterprojecttypes"},
		Short:   "Manage cluster project types",
		Long:    `Manage cluster-scoped project types for OpenChoreo.`,
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
		Short: "List cluster project types",
		Long:  `List all cluster-scoped project types available across the cluster.`,
		Example: `  # List all cluster project types
  occ clusterprojecttype list`,
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
		Use:   "get [CLUSTER_PROJECT_TYPE_NAME]",
		Short: "Get a cluster project type",
		Long:  `Get a cluster project type and display its details in YAML format.`,
		Example: `  # Get a cluster project type
  occ clusterprojecttype get default`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{ClusterProjectTypeName: args[0]})
		},
	}
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "delete [CLUSTER_PROJECT_TYPE_NAME]",
		Short: "Delete a cluster project type",
		Long:  `Delete a cluster project type by name.`,
		Example: `  # Delete a cluster project type
  occ clusterprojecttype delete default`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{ClusterProjectTypeName: args[0]})
		},
	}
}
