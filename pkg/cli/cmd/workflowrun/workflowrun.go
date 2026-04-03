// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflowrun"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	apiclient "github.com/openchoreo/openchoreo/pkg/cli/common/client"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewWorkflowRunCmd() *cobra.Command {
	workflowRunCmd := &cobra.Command{
		Use:     constants.WorkflowRun.Use,
		Aliases: constants.WorkflowRun.Aliases,
		Short:   constants.WorkflowRun.Short,
		Long:    constants.WorkflowRun.Long,
	}

	workflowRunCmd.AddCommand(
		newListWorkflowRunCmd(),
		newGetWorkflowRunCmd(),
		newLogsWorkflowRunCmd(),
	)

	return workflowRunCmd
}

func newListWorkflowRunCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListWorkflowRun,
		Flags: []flags.Flag{
			flags.Namespace,
			flags.Workflow,
		},
		RunE: func(fg *builder.FlagGetter) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return workflowrun.New(cl).List(workflowrun.ListParams{
				Namespace: fg.GetString(flags.Namespace),
				Workflow:  fg.GetString(flags.Workflow),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetWorkflowRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetWorkflowRun.Use,
		Short:   constants.GetWorkflowRun.Short,
		Long:    constants.GetWorkflowRun.Long,
		Example: constants.GetWorkflowRun.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return workflowrun.New(cl).Get(workflowrun.GetParams{
				Namespace:       namespace,
				WorkflowRunName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}

func newLogsWorkflowRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.LogsWorkflowRun.Use,
		Short:   constants.LogsWorkflowRun.Short,
		Long:    constants.LogsWorkflowRun.Long,
		Example: constants.LogsWorkflowRun.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			follow, _ := cmd.Flags().GetBool(flags.Follow.Name)
			since, _ := cmd.Flags().GetString(flags.Since.Name)
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return workflowrun.New(cl).Logs(workflowrun.LogsParams{
				Namespace:       namespace,
				WorkflowRunName: args[0],
				Follow:          follow,
				Since:           since,
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace, flags.Follow, flags.Since)
	return cmd
}
