// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secretreference

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewSecretReferenceCmd(impl api.CommandImplementationInterface) *cobra.Command {
	secretReferenceCmd := &cobra.Command{
		Use:     constants.SecretReference.Use,
		Aliases: constants.SecretReference.Aliases,
		Short:   constants.SecretReference.Short,
		Long:    constants.SecretReference.Long,
	}

	secretReferenceCmd.AddCommand(
		newListSecretReferenceCmd(impl),
	)

	return secretReferenceCmd
}

func newListSecretReferenceCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListSecretReference,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListSecretReferences(api.ListSecretReferencesParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
	}).Build()
}
