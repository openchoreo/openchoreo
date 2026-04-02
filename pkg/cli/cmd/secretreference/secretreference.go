// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secretreference

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/secretreference"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	apiclient "github.com/openchoreo/openchoreo/pkg/cli/common/client"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewSecretReferenceCmd() *cobra.Command {
	secretReferenceCmd := &cobra.Command{
		Use:     constants.SecretReference.Use,
		Aliases: constants.SecretReference.Aliases,
		Short:   constants.SecretReference.Short,
		Long:    constants.SecretReference.Long,
	}

	secretReferenceCmd.AddCommand(
		newListSecretReferenceCmd(),
		newGetSecretReferenceCmd(),
		newDeleteSecretReferenceCmd(),
	)

	return secretReferenceCmd
}

func newListSecretReferenceCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListSecretReference,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return secretreference.New(cl).List(secretreference.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetSecretReferenceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetSecretReference.Use,
		Short:   constants.GetSecretReference.Short,
		Long:    constants.GetSecretReference.Long,
		Example: constants.GetSecretReference.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return secretreference.New(cl).Get(secretreference.GetParams{
				Namespace:           namespace,
				SecretReferenceName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}

func newDeleteSecretReferenceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteSecretReference.Use,
		Short:   constants.DeleteSecretReference.Short,
		Long:    constants.DeleteSecretReference.Long,
		Example: constants.DeleteSecretReference.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return secretreference.New(cl).Delete(secretreference.DeleteParams{
				Namespace:           namespace,
				SecretReferenceName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}
