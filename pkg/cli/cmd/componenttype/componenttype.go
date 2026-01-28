// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewComponentTypeCmd(impl api.CommandImplementationInterface) *cobra.Command {
	componentTypeCmd := &cobra.Command{
		Use:     constants.ComponentType.Use,
		Aliases: constants.ComponentType.Aliases,
		Short:   constants.ComponentType.Short,
		Long:    constants.ComponentType.Long,
	}

	componentTypeCmd.AddCommand(
		newListComponentTypeCmd(impl),
	)

	return componentTypeCmd
}

func newListComponentTypeCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListComponentType,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListComponentTypes(api.ListComponentTypesParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
	}).Build()
}
