// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflowrun"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
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
	)

	return workflowRunCmd
}

func newListWorkflowRunCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListWorkflowRun,
		Flags: []flags.Flag{
			flags.Namespace,
		},
		RunE: func(fg *builder.FlagGetter) error {
			return workflowrun.New().List(workflowrun.ListParams{
				Namespace: fg.GetString(flags.Namespace),
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
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return workflowrun.New().Get(workflowrun.GetParams{
				Namespace:       namespace,
				WorkflowRunName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}
