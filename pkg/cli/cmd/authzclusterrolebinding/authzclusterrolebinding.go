// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzclusterrolebinding

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/authzclusterrolebinding"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

// NewAuthzClusterRoleBindingCmd returns the parent command for authz cluster role binding operations
func NewAuthzClusterRoleBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.AuthzClusterRoleBinding.Use,
		Aliases: constants.AuthzClusterRoleBinding.Aliases,
		Short:   constants.AuthzClusterRoleBinding.Short,
		Long:    constants.AuthzClusterRoleBinding.Long,
	}

	cmd.AddCommand(
		newListAuthzClusterRoleBindingCmd(),
		newGetAuthzClusterRoleBindingCmd(),
		newDeleteAuthzClusterRoleBindingCmd(),
	)

	return cmd
}

func newListAuthzClusterRoleBindingCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListAuthzClusterRoleBinding,
		Flags:   []flags.Flag{},
		RunE: func(fg *builder.FlagGetter) error {
			return authzclusterrolebinding.New().List()
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetAuthzClusterRoleBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetAuthzClusterRoleBinding.Use,
		Short:   constants.GetAuthzClusterRoleBinding.Short,
		Long:    constants.GetAuthzClusterRoleBinding.Long,
		Example: constants.GetAuthzClusterRoleBinding.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return authzclusterrolebinding.New().Get(authzclusterrolebinding.GetParams{
				Name: args[0],
			})
		},
	}

	return cmd
}

func newDeleteAuthzClusterRoleBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteAuthzClusterRoleBinding.Use,
		Short:   constants.DeleteAuthzClusterRoleBinding.Short,
		Long:    constants.DeleteAuthzClusterRoleBinding.Long,
		Example: constants.DeleteAuthzClusterRoleBinding.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return authzclusterrolebinding.New().Delete(authzclusterrolebinding.DeleteParams{
				Name: args[0],
			})
		},
	}

	return cmd
}
