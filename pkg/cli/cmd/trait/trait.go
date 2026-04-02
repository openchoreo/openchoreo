// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/trait"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	apiclient "github.com/openchoreo/openchoreo/pkg/cli/common/client"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewTraitCmd() *cobra.Command {
	traitCmd := &cobra.Command{
		Use:     constants.Trait.Use,
		Aliases: constants.Trait.Aliases,
		Short:   constants.Trait.Short,
		Long:    constants.Trait.Long,
	}

	traitCmd.AddCommand(
		newListTraitCmd(),
		newGetTraitCmd(),
		newDeleteTraitCmd(),
	)

	return traitCmd
}

func newGetTraitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetTrait.Use,
		Short:   constants.GetTrait.Short,
		Long:    constants.GetTrait.Long,
		Example: constants.GetTrait.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return trait.New(cl).Get(trait.GetParams{
				Namespace: namespace,
				TraitName: args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}

func newDeleteTraitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteTrait.Use,
		Short:   constants.DeleteTrait.Short,
		Long:    constants.DeleteTrait.Long,
		Example: constants.DeleteTrait.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return trait.New(cl).Delete(trait.DeleteParams{
				Namespace: namespace,
				TraitName: args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}

func newListTraitCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListTrait,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return trait.New(cl).List(trait.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
