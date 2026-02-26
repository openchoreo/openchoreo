// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrolebinding

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/authzrolebinding"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

// NewAuthzRoleBindingCmd returns the parent command for authz role binding operations
func NewAuthzRoleBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.AuthzRoleBinding.Use,
		Aliases: constants.AuthzRoleBinding.Aliases,
		Short:   constants.AuthzRoleBinding.Short,
		Long:    constants.AuthzRoleBinding.Long,
	}

	cmd.AddCommand(
		newListAuthzRoleBindingCmd(),
		newGetAuthzRoleBindingCmd(),
		newDeleteAuthzRoleBindingCmd(),
	)

	return cmd
}

func newListAuthzRoleBindingCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListAuthzRoleBinding,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return authzrolebinding.New().List(authzrolebinding.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetAuthzRoleBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetAuthzRoleBinding.Use,
		Short:   constants.GetAuthzRoleBinding.Short,
		Long:    constants.GetAuthzRoleBinding.Long,
		Example: constants.GetAuthzRoleBinding.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)

			return authzrolebinding.New().Get(authzrolebinding.GetParams{
				Namespace: namespace,
				Name:      args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}

func newDeleteAuthzRoleBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteAuthzRoleBinding.Use,
		Short:   constants.DeleteAuthzRoleBinding.Short,
		Long:    constants.DeleteAuthzRoleBinding.Long,
		Example: constants.DeleteAuthzRoleBinding.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)

			return authzrolebinding.New().Delete(authzrolebinding.DeleteParams{
				Namespace: namespace,
				Name:      args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}
