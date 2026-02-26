// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrole

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/authzrole"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

// NewAuthzRoleCmd returns the parent command for authz role operations
func NewAuthzRoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.AuthzRole.Use,
		Aliases: constants.AuthzRole.Aliases,
		Short:   constants.AuthzRole.Short,
		Long:    constants.AuthzRole.Long,
	}

	cmd.AddCommand(
		newListAuthzRoleCmd(),
		newGetAuthzRoleCmd(),
		newDeleteAuthzRoleCmd(),
	)

	return cmd
}

func newListAuthzRoleCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListAuthzRole,
		Flags:   []flags.Flag{flags.Namespace},
		RunE: func(fg *builder.FlagGetter) error {
			return authzrole.New().List(authzrole.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetAuthzRoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetAuthzRole.Use,
		Short:   constants.GetAuthzRole.Short,
		Long:    constants.GetAuthzRole.Long,
		Example: constants.GetAuthzRole.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)

			return authzrole.New().Get(authzrole.GetParams{
				Namespace: namespace,
				Name:      args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}

func newDeleteAuthzRoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteAuthzRole.Use,
		Short:   constants.DeleteAuthzRole.Short,
		Long:    constants.DeleteAuthzRole.Long,
		Example: constants.DeleteAuthzRole.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)

			return authzrole.New().Delete(authzrole.DeleteParams{
				Namespace: namespace,
				Name:      args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}
