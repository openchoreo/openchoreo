// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clustercomponenttype"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewClusterComponentTypeCmd() *cobra.Command {
	clusterComponentTypeCmd := &cobra.Command{
		Use:     constants.ClusterComponentType.Use,
		Aliases: constants.ClusterComponentType.Aliases,
		Short:   constants.ClusterComponentType.Short,
		Long:    constants.ClusterComponentType.Long,
	}

	clusterComponentTypeCmd.AddCommand(
		newListClusterComponentTypeCmd(),
		newGetClusterComponentTypeCmd(),
		newDeleteClusterComponentTypeCmd(),
	)

	return clusterComponentTypeCmd
}

func newGetClusterComponentTypeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetClusterComponentType.Use,
		Short:   constants.GetClusterComponentType.Short,
		Long:    constants.GetClusterComponentType.Long,
		Example: constants.GetClusterComponentType.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clustercomponenttype.New().Get(clustercomponenttype.GetParams{
				ClusterComponentTypeName: args[0],
			})
		},
	}
	return cmd
}

func newDeleteClusterComponentTypeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteClusterComponentType.Use,
		Short:   constants.DeleteClusterComponentType.Short,
		Long:    constants.DeleteClusterComponentType.Long,
		Example: constants.DeleteClusterComponentType.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clustercomponenttype.New().Delete(clustercomponenttype.DeleteParams{
				ClusterComponentTypeName: args[0],
			})
		},
	}
	return cmd
}

func newListClusterComponentTypeCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListClusterComponentType,
		Flags:   []flags.Flag{},
		RunE: func(fg *builder.FlagGetter) error {
			return clustercomponenttype.New().List()
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
