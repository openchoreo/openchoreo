// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewWorkflowCmd(impl api.CommandImplementationInterface) *cobra.Command {
	workflowCmd := &cobra.Command{
		Use:     constants.Workflow.Use,
		Aliases: constants.Workflow.Aliases,
		Short:   constants.Workflow.Short,
		Long:    constants.Workflow.Long,
	}

	workflowCmd.AddCommand(
		newListWorkflowCmd(impl),
		newStartWorkflowCmd(impl),
	)

	return workflowCmd
}

func newListWorkflowCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListWorkflow,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListWorkflows(api.ListWorkflowsParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(impl),
	}).Build()
}

func newStartWorkflowCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.StartWorkflow,
		Flags:   []flags.Flag{flags.Namespace, flags.Set},
		RunE: func(fg *builder.FlagGetter) error {
			workflowName := fg.GetArgs()[0]
			return impl.StartWorkflowRun(api.StartWorkflowRunParams{
				Namespace:    fg.GetString(flags.Namespace),
				WorkflowName: workflowName,
				Parameters:   fg.GetStringArray(flags.Set),
			})
		},
		PreRunE: auth.RequireLogin(impl),
	}).Build()
	cmd.Args = cobra.ExactArgs(1) // Require workflow name as positional arg
	return cmd
}
