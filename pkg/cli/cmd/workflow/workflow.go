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
