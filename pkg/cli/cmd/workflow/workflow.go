// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflow"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	apiclient "github.com/openchoreo/openchoreo/pkg/cli/common/client"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewWorkflowCmd() *cobra.Command {
	workflowCmd := &cobra.Command{
		Use:     constants.Workflow.Use,
		Aliases: constants.Workflow.Aliases,
		Short:   constants.Workflow.Short,
		Long:    constants.Workflow.Long,
	}

	workflowCmd.AddCommand(
		newListWorkflowCmd(),
		newGetWorkflowCmd(),
		newDeleteWorkflowCmd(),
		newStartWorkflowCmd(),
		newLogsWorkflowCmd(),
	)

	return workflowCmd
}

func newListWorkflowCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListWorkflow,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return workflow.New(cl).List(workflow.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetWorkflow.Use,
		Short:   constants.GetWorkflow.Short,
		Long:    constants.GetWorkflow.Long,
		Example: constants.GetWorkflow.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return workflow.New(cl).Get(workflow.GetParams{
				Namespace:    namespace,
				WorkflowName: args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}

func newDeleteWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteWorkflow.Use,
		Short:   constants.DeleteWorkflow.Short,
		Long:    constants.DeleteWorkflow.Long,
		Example: constants.DeleteWorkflow.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return workflow.New(cl).Delete(workflow.DeleteParams{
				Namespace:    namespace,
				WorkflowName: args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}

func newStartWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.StartWorkflow.Use,
		Short:   constants.StartWorkflow.Short,
		Long:    constants.StartWorkflow.Long,
		Example: constants.StartWorkflow.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			set, _ := cmd.Flags().GetStringArray(flags.Set.Name)
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return workflow.New(cl).StartRun(workflow.StartRunParams{
				Namespace:    namespace,
				WorkflowName: args[0],
				Set:          set,
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace, flags.Set)

	return cmd
}

func newLogsWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.LogsWorkflow.Use,
		Short:   constants.LogsWorkflow.Short,
		Long:    constants.LogsWorkflow.Long,
		Example: constants.LogsWorkflow.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			follow, _ := cmd.Flags().GetBool(flags.Follow.Name)
			since, _ := cmd.Flags().GetString(flags.Since.Name)
			run, _ := cmd.Flags().GetString(flags.WorkflowRun.Name)
			return workflow.New(nil).Logs(workflow.LogsParams{
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
