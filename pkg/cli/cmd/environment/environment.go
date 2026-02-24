// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/environment"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewEnvironmentCmd() *cobra.Command {
	environmentCmd := &cobra.Command{
		Use:     constants.Environment.Use,
		Aliases: constants.Environment.Aliases,
		Short:   constants.Environment.Short,
		Long:    constants.Environment.Long,
	}

	environmentCmd.AddCommand(
		newListEnvironmentCmd(),
	)

	return environmentCmd
}

func newListEnvironmentCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListEnvironment,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return environment.New().List(environment.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
