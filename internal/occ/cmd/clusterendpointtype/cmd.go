// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterendpointtype

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewClusterEndpointTypeCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clusterendpointtype",
		Aliases: []string{"cet", "clusterendpointtypes"},
		Short:   "Manage cluster endpoint types",
		Long:    `Manage cluster-scoped endpoint types for OpenChoreo.`,
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
		Short: "List cluster endpoint types",
		Long:  `List all cluster-scoped endpoint types available across the cluster.`,
		Example: `  # List all cluster endpoint types
  occ clusterendpointtype list`,
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
		Use:   "get [CLUSTER_ENDPOINT_TYPE_NAME]",
		Short: "Get a cluster endpoint type",
		Long:  `Get a cluster endpoint type and display its details in YAML format.`,
		Example: `  # Get a cluster endpoint type
  occ clusterendpointtype get public-http`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{ClusterEndpointTypeName: args[0]})
		},
	}
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "delete [CLUSTER_ENDPOINT_TYPE_NAME]",
		Short: "Delete a cluster endpoint type",
		Long:  `Delete a cluster endpoint type by name.`,
		Example: `  # Delete a cluster endpoint type
  occ clusterendpointtype delete public-http`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{ClusterEndpointTypeName: args[0]})
		},
	}
}
