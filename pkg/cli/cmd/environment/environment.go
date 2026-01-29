// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewEnvironmentCmd(impl api.CommandImplementationInterface) *cobra.Command {
	environmentCmd := &cobra.Command{
		Use:     constants.Environment.Use,
		Aliases: constants.Environment.Aliases,
		Short:   constants.Environment.Short,
		Long:    constants.Environment.Long,
	}

	environmentCmd.AddCommand(
		newListEnvironmentCmd(impl),
	)

	return environmentCmd
}

func newListEnvironmentCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListEnvironment,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListEnvironments(api.ListEnvironmentsParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(impl),
	}).Build()
}
