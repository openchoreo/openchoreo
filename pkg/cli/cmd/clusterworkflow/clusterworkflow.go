// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterworkflow"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewClusterWorkflowCmd() *cobra.Command {
	clusterWorkflowCmd := &cobra.Command{
		Use:     constants.ClusterWorkflow.Use,
		Aliases: constants.ClusterWorkflow.Aliases,
		Short:   constants.ClusterWorkflow.Short,
		Long:    constants.ClusterWorkflow.Long,
	}

	clusterWorkflowCmd.AddCommand(
		newListClusterWorkflowCmd(),
		newGetClusterWorkflowCmd(),
		newDeleteClusterWorkflowCmd(),
	)

	return clusterWorkflowCmd
}

func newGetClusterWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetClusterWorkflow.Use,
		Short:   constants.GetClusterWorkflow.Short,
		Long:    constants.GetClusterWorkflow.Long,
		Example: constants.GetClusterWorkflow.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clusterworkflow.New().Get(clusterworkflow.GetParams{
				ClusterWorkflowName: args[0],
			})
		},
	}
	return cmd
}

func newDeleteClusterWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteClusterWorkflow.Use,
		Short:   constants.DeleteClusterWorkflow.Short,
		Long:    constants.DeleteClusterWorkflow.Long,
		Example: constants.DeleteClusterWorkflow.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clusterworkflow.New().Delete(clusterworkflow.DeleteParams{
				ClusterWorkflowName: args[0],
			})
		},
	}
	return cmd
}

func newListClusterWorkflowCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListClusterWorkflow,
		Flags:   []flags.Flag{},
		RunE: func(fg *builder.FlagGetter) error {
			return clusterworkflow.New().List()
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
