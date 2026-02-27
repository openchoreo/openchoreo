// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflow"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
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
		newStartWorkflowCmd(),
	)

	return workflowCmd
}

func newListWorkflowCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListWorkflow,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return workflow.New().List(workflow.ListParams{
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
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return workflow.New().Get(workflow.GetParams{
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
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			set, _ := cmd.Flags().GetStringArray(flags.Set.Name)
			return workflow.New().StartRun(workflow.StartRunParams{
				Namespace:    namespace,
				WorkflowName: args[0],
				Set:          set,
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace, flags.Set)

	return cmd
}
