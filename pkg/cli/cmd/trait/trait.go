// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewTraitCmd(impl api.CommandImplementationInterface) *cobra.Command {
	traitCmd := &cobra.Command{
		Use:     constants.Trait.Use,
		Aliases: constants.Trait.Aliases,
		Short:   constants.Trait.Short,
		Long:    constants.Trait.Long,
	}

	traitCmd.AddCommand(
		newListTraitCmd(impl),
	)

	return traitCmd
}

func newListTraitCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListTrait,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListTraits(api.ListTraitsParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
	}).Build()
}
