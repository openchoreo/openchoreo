// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflowplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterworkflowplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	apiclient "github.com/openchoreo/openchoreo/pkg/cli/common/client"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewClusterWorkflowPlaneCmd() *cobra.Command {
	clusterWorkflowPlaneCmd := &cobra.Command{
		Use:     constants.ClusterWorkflowPlane.Use,
		Aliases: constants.ClusterWorkflowPlane.Aliases,
		Short:   constants.ClusterWorkflowPlane.Short,
		Long:    constants.ClusterWorkflowPlane.Long,
	}

	clusterWorkflowPlaneCmd.AddCommand(
		newListClusterWorkflowPlaneCmd(),
		newGetClusterWorkflowPlaneCmd(),
		newDeleteClusterWorkflowPlaneCmd(),
	)

	return clusterWorkflowPlaneCmd
}

func newGetClusterWorkflowPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetClusterWorkflowPlane.Use,
		Short:   constants.GetClusterWorkflowPlane.Short,
		Long:    constants.GetClusterWorkflowPlane.Long,
		Example: constants.GetClusterWorkflowPlane.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return clusterworkflowplane.New(cl).Get(clusterworkflowplane.GetParams{
				ClusterWorkflowPlaneName: args[0],
			})
		},
	}
	return cmd
}

func newDeleteClusterWorkflowPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteClusterWorkflowPlane.Use,
		Short:   constants.DeleteClusterWorkflowPlane.Short,
		Long:    constants.DeleteClusterWorkflowPlane.Long,
		Example: constants.DeleteClusterWorkflowPlane.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return clusterworkflowplane.New(cl).Delete(clusterworkflowplane.DeleteParams{
				ClusterWorkflowPlaneName: args[0],
			})
		},
	}
	return cmd
}

func newListClusterWorkflowPlaneCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListClusterWorkflowPlane,
		Flags:   []flags.Flag{},
		RunE: func(fg *builder.FlagGetter) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return clusterworkflowplane.New(cl).List()
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
