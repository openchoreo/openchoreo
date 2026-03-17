// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterobservabilityplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewClusterObservabilityPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.ClusterObservabilityPlane.Use,
		Aliases: constants.ClusterObservabilityPlane.Aliases,
		Short:   constants.ClusterObservabilityPlane.Short,
		Long:    constants.ClusterObservabilityPlane.Long,
	}

	cmd.AddCommand(
		newListClusterObservabilityPlaneCmd(),
		newGetClusterObservabilityPlaneCmd(),
		newDeleteClusterObservabilityPlaneCmd(),
	)

	return cmd
}

func newGetClusterObservabilityPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetClusterObservabilityPlane.Use,
		Short:   constants.GetClusterObservabilityPlane.Short,
		Long:    constants.GetClusterObservabilityPlane.Long,
		Example: constants.GetClusterObservabilityPlane.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clusterobservabilityplane.New().Get(clusterobservabilityplane.GetParams{
				ClusterObservabilityPlaneName: args[0],
			})
		},
	}
	return cmd
}

func newDeleteClusterObservabilityPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteClusterObservabilityPlane.Use,
		Short:   constants.DeleteClusterObservabilityPlane.Short,
		Long:    constants.DeleteClusterObservabilityPlane.Long,
		Example: constants.DeleteClusterObservabilityPlane.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clusterobservabilityplane.New().Delete(clusterobservabilityplane.DeleteParams{
				ClusterObservabilityPlaneName: args[0],
			})
		},
	}
	return cmd
}

func newListClusterObservabilityPlaneCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListClusterObservabilityPlane,
		Flags:   []flags.Flag{},
		RunE: func(fg *builder.FlagGetter) error {
			return clusterobservabilityplane.New().List()
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
