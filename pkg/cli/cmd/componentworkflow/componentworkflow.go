// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentworkflow

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewComponentWorkflowCmd(impl api.CommandImplementationInterface) *cobra.Command {
	componentWorkflowCmd := &cobra.Command{
		Use:     constants.ComponentWorkflow.Use,
		Aliases: constants.ComponentWorkflow.Aliases,
		Short:   constants.ComponentWorkflow.Short,
		Long:    constants.ComponentWorkflow.Long,
	}

	componentWorkflowCmd.AddCommand(
		newListComponentWorkflowCmd(impl),
	)

	return componentWorkflowCmd
}

func newListComponentWorkflowCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListComponentWorkflow,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListComponentWorkflows(api.ListComponentWorkflowsParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(impl),
	}).Build()
}
