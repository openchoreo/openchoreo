// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/observabilityplane"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewObservabilityPlaneCmd() *cobra.Command {
	observabilityPlaneCmd := &cobra.Command{
		Use:     constants.ObservabilityPlane.Use,
		Aliases: constants.ObservabilityPlane.Aliases,
		Short:   constants.ObservabilityPlane.Short,
		Long:    constants.ObservabilityPlane.Long,
	}

	observabilityPlaneCmd.AddCommand(
		newListObservabilityPlaneCmd(),
	)

	return observabilityPlaneCmd
}

func newListObservabilityPlaneCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListObservabilityPlane,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return observabilityplane.New().List(observabilityplane.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
