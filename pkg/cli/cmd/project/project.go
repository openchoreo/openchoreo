// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewProjectCmd(impl api.CommandImplementationInterface) *cobra.Command {
	projectCmd := &cobra.Command{
		Use:     constants.Project.Use,
		Aliases: constants.Project.Aliases,
		Short:   constants.Project.Short,
		Long:    constants.Project.Long,
	}

	projectCmd.AddCommand(
		newListProjectCmd(impl),
	)

	return projectCmd
}

func newListProjectCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListProject,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListProjects(api.ListProjectsParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
	}).Build()
}
