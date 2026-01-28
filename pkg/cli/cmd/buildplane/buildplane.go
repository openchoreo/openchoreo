// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewBuildPlaneCmd(impl api.CommandImplementationInterface) *cobra.Command {
	buildPlaneCmd := &cobra.Command{
		Use:     constants.BuildPlane.Use,
		Aliases: constants.BuildPlane.Aliases,
		Short:   constants.BuildPlane.Short,
		Long:    constants.BuildPlane.Long,
	}

	buildPlaneCmd.AddCommand(
		newListBuildPlaneCmd(impl),
	)

	return buildPlaneCmd
}

func newListBuildPlaneCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListBuildPlane,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListBuildPlanes(api.ListBuildPlanesParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
	}).Build()
}
