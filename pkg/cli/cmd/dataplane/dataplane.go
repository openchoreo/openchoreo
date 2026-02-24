// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/dataplane"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewDataPlaneCmd() *cobra.Command {
	dataPlaneCmd := &cobra.Command{
		Use:     constants.DataPlane.Use,
		Aliases: constants.DataPlane.Aliases,
		Short:   constants.DataPlane.Short,
		Long:    constants.DataPlane.Long,
	}

	dataPlaneCmd.AddCommand(
		newListDataPlaneCmd(),
	)

	return dataPlaneCmd
}

func newListDataPlaneCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListDataPlane,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return dataplane.New().List(dataplane.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
