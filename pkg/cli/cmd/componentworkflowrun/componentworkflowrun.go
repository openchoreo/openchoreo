// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentworkflowrun

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewComponentWorkflowRunCmd(impl api.CommandImplementationInterface) *cobra.Command {
	componentWorkflowRunCmd := &cobra.Command{
		Use:     constants.ComponentWorkflowRun.Use,
		Aliases: constants.ComponentWorkflowRun.Aliases,
		Short:   constants.ComponentWorkflowRun.Short,
		Long:    constants.ComponentWorkflowRun.Long,
	}

	componentWorkflowRunCmd.AddCommand(
		newListComponentWorkflowRunCmd(impl),
	)

	return componentWorkflowRunCmd
}

func newListComponentWorkflowRunCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListComponentWorkflowRun,
		Flags: []flags.Flag{
			flags.Namespace,
			flags.Project,
			flags.Component,
		},
		RunE: func(fg *builder.FlagGetter) error {
			params := api.ListComponentWorkflowRunsParams{
				Namespace: fg.GetString(flags.Namespace),
				Project:   fg.GetString(flags.Project),
				Component: fg.GetString(flags.Component),
			}
			return impl.ListComponentWorkflowRuns(params)
		},
		PreRunE: auth.RequireLogin(impl),
	}).Build()
}
