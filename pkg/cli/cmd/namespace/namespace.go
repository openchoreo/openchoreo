// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package namespace

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewNamespaceCmd(impl api.CommandImplementationInterface) *cobra.Command {
	namespaceCmd := &cobra.Command{
		Use:     constants.Namespace.Use,
		Aliases: constants.Namespace.Aliases,
		Short:   constants.Namespace.Short,
		Long:    constants.Namespace.Long,
	}

	namespaceCmd.AddCommand(
		newListNamespaceCmd(impl),
	)

	return namespaceCmd
}

func newListNamespaceCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListNamespace,
		Flags:   []flags.Flag{},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListNamespaces(api.ListNamespacesParams{})
		},
		PreRunE: auth.RequireLogin(impl),
	}).Build()
}
