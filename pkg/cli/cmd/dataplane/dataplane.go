// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewDataPlaneCmd(impl api.CommandImplementationInterface) *cobra.Command {
	dataPlaneCmd := &cobra.Command{
		Use:     constants.DataPlane.Use,
		Aliases: constants.DataPlane.Aliases,
		Short:   constants.DataPlane.Short,
		Long:    constants.DataPlane.Long,
	}

	dataPlaneCmd.AddCommand(
		newListDataPlaneCmd(impl),
	)

	return dataPlaneCmd
}

func newListDataPlaneCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListDataPlane,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListDataPlanes(api.ListDataPlanesParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
	}).Build()
}
