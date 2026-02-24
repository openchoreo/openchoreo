// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/buildplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewBuildPlaneCmd() *cobra.Command {
	buildPlaneCmd := &cobra.Command{
		Use:     constants.BuildPlane.Use,
		Aliases: constants.BuildPlane.Aliases,
		Short:   constants.BuildPlane.Short,
		Long:    constants.BuildPlane.Long,
	}

	buildPlaneCmd.AddCommand(
		newListBuildPlaneCmd(),
	)

	return buildPlaneCmd
}

func newListBuildPlaneCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListBuildPlane,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return buildplane.New().List(buildplane.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
