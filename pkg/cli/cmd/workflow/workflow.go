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

func newStartWorkflowCmd() *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.StartWorkflow,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			workflowName := fg.GetArgs()[0]
			return workflow.New().StartRun(workflow.StartRunParams{
				Namespace:    fg.GetString(flags.Namespace),
				WorkflowName: workflowName,
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}
