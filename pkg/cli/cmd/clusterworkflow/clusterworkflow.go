// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterworkflow"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
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
		newStartClusterWorkflowCmd(),
		newLogsClusterWorkflowCmd(),
	)

	return clusterWorkflowCmd
}

func newGetClusterWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetClusterWorkflow.Use,
		Short:   constants.GetClusterWorkflow.Short,
		Long:    constants.GetClusterWorkflow.Long,
		Example: constants.GetClusterWorkflow.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
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
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clusterworkflow.New().Delete(clusterworkflow.DeleteParams{
				ClusterWorkflowName: args[0],
			})
		},
	}
	return cmd
}

func newStartClusterWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.StartClusterWorkflow.Use,
		Short:   constants.StartClusterWorkflow.Short,
		Long:    constants.StartClusterWorkflow.Long,
		Example: constants.StartClusterWorkflow.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			set, _ := cmd.Flags().GetStringArray(flags.Set.Name)
			return clusterworkflow.New().StartRun(clusterworkflow.StartRunParams{
				Namespace:    namespace,
				WorkflowName: args[0],
				Set:          set,
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace, flags.Set)

	return cmd
}

func newLogsClusterWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.LogsClusterWorkflow.Use,
		Short:   constants.LogsClusterWorkflow.Short,
		Long:    constants.LogsClusterWorkflow.Long,
		Example: constants.LogsClusterWorkflow.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			follow, _ := cmd.Flags().GetBool(flags.Follow.Name)
			since, _ := cmd.Flags().GetString(flags.Since.Name)
			run, _ := cmd.Flags().GetString(flags.WorkflowRun.Name)
			return clusterworkflow.New().Logs(clusterworkflow.LogsParams{
				Namespace:    namespace,
				WorkflowName: args[0],
				RunName:      run,
				Follow:       follow,
				Since:        since,
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace, flags.Follow, flags.Since, flags.WorkflowRun)

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
