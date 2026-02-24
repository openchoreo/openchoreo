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
