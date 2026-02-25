// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package namespace

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/namespace"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewNamespaceCmd() *cobra.Command {
	namespaceCmd := &cobra.Command{
		Use:     constants.Namespace.Use,
		Aliases: constants.Namespace.Aliases,
		Short:   constants.Namespace.Short,
		Long:    constants.Namespace.Long,
	}

	namespaceCmd.AddCommand(
		newListNamespaceCmd(),
		newGetNamespaceCmd(),
		newDeleteNamespaceCmd(),
	)

	return namespaceCmd
}

func newListNamespaceCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListNamespace,
		Flags:   []flags.Flag{},
		RunE: func(fg *builder.FlagGetter) error {
			return namespace.New().List()
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetNamespaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetNamespace.Use,
		Short:   constants.GetNamespace.Short,
		Long:    constants.GetNamespace.Long,
		Example: constants.GetNamespace.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return namespace.New().Get(args[0])
		},
	}

	return cmd
}

func newDeleteNamespaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteNamespace.Use,
		Short:   constants.DeleteNamespace.Short,
		Long:    constants.DeleteNamespace.Long,
		Example: constants.DeleteNamespace.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return namespace.New().Delete(args[0])
		},
	}

	return cmd
}
