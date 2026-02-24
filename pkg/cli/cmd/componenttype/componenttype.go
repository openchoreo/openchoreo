// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/componenttype"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewComponentTypeCmd() *cobra.Command {
	componentTypeCmd := &cobra.Command{
		Use:     constants.ComponentType.Use,
		Aliases: constants.ComponentType.Aliases,
		Short:   constants.ComponentType.Short,
		Long:    constants.ComponentType.Long,
	}

	componentTypeCmd.AddCommand(
		newListComponentTypeCmd(),
	)

	return componentTypeCmd
}

func newListComponentTypeCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListComponentType,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return componenttype.New().List(componenttype.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
