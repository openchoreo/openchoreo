// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewWorkflowRunCmd(impl api.CommandImplementationInterface) *cobra.Command {
	workflowRunCmd := &cobra.Command{
		Use:     constants.WorkflowRun.Use,
		Aliases: constants.WorkflowRun.Aliases,
		Short:   constants.WorkflowRun.Short,
		Long:    constants.WorkflowRun.Long,
	}

	workflowRunCmd.AddCommand(
		newListWorkflowRunCmd(impl),
	)

	return workflowRunCmd
}

func newListWorkflowRunCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListWorkflowRun,
		Flags: []flags.Flag{
			flags.Namespace,
		},
		RunE: func(fg *builder.FlagGetter) error {
			params := api.ListWorkflowRunsParams{
				Namespace: fg.GetString(flags.Namespace),
			}
			return impl.ListWorkflowRuns(params)
		},
		PreRunE: auth.RequireLogin(impl),
	}).Build()
}
