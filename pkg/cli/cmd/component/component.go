// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewComponentCmd(impl api.CommandImplementationInterface) *cobra.Command {
	componentCmd := &cobra.Command{
		Use:     constants.Component.Use,
		Aliases: constants.Component.Aliases,
		Short:   constants.Component.Short,
		Long:    constants.Component.Long,
	}

	componentCmd.AddCommand(
		newListComponentCmd(impl),
	)

	return componentCmd
}

func newListComponentCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListComponent,
		Flags:   []flags.Flag{flags.Namespace, flags.Project},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListComponents(api.ListComponentsParams{
				Namespace: fg.GetString(flags.Namespace),
				Project:   fg.GetString(flags.Project),
			})
		},
	}).Build()
}
