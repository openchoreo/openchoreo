// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
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
		newScaffoldComponentCmd(impl),
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

func newScaffoldComponentCmd(impl api.CommandImplementationInterface) *cobra.Command {
	componentFlags := []flags.Flag{
		flags.Name,
		flags.ScaffoldType,
		flags.Traits,
		flags.Workflow,
		flags.Project,
		flags.Namespace,
		flags.OutputFile,
		flags.SkipComments,
		flags.SkipOptional,
	}

	return (&builder.CommandBuilder{
		Command: constants.ScaffoldComponent,
		Flags:   componentFlags,
		PreRunE: auth.RequireLogin(impl),
		RunE: func(fg *builder.FlagGetter) error {
			// Parse traits from comma-separated string
			traitsStr := fg.GetString(flags.Traits)
			var traits []string
			if traitsStr != "" {
				parts := strings.Split(traitsStr, ",")
				for _, part := range parts {
					trimmed := strings.TrimSpace(part)
					if trimmed != "" {
						traits = append(traits, trimmed)
					}
				}
			}

			return impl.ScaffoldComponent(api.ScaffoldComponentParams{
				ComponentName: fg.GetString(flags.Name),
				ComponentType: fg.GetString(flags.ScaffoldType),
				Traits:        traits,
				WorkflowName:  fg.GetString(flags.Workflow),
				Namespace:     fg.GetString(flags.Namespace),
				ProjectName:   fg.GetString(flags.Project),
				OutputPath:    fg.GetString(flags.OutputFile),
				SkipComments:  fg.GetBool(flags.SkipComments),
				SkipOptional:  fg.GetBool(flags.SkipOptional),
			})
		},
	}).Build()
}
