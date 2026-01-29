// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewObservabilityPlaneCmd(impl api.CommandImplementationInterface) *cobra.Command {
	observabilityPlaneCmd := &cobra.Command{
		Use:     constants.ObservabilityPlane.Use,
		Aliases: constants.ObservabilityPlane.Aliases,
		Short:   constants.ObservabilityPlane.Short,
		Long:    constants.ObservabilityPlane.Long,
	}

	observabilityPlaneCmd.AddCommand(
		newListObservabilityPlaneCmd(impl),
	)

	return observabilityPlaneCmd
}

func newListObservabilityPlaneCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListObservabilityPlane,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListObservabilityPlanes(api.ListObservabilityPlanesParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(impl),
	}).Build()
}
