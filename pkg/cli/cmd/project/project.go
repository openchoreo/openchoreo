// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/project"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewProjectCmd() *cobra.Command {
	projectCmd := &cobra.Command{
		Use:     constants.Project.Use,
		Aliases: constants.Project.Aliases,
		Short:   constants.Project.Short,
		Long:    constants.Project.Long,
	}

	projectCmd.AddCommand(
		newListProjectCmd(),
	)

	return projectCmd
}

func newListProjectCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListProject,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			projectImpl := project.New()
			return projectImpl.List(project.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
