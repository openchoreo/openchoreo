// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzclusterrole

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/authzclusterrole"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

// NewAuthzClusterRoleCmd returns the parent command for authz cluster role operations
func NewAuthzClusterRoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.AuthzClusterRole.Use,
		Aliases: constants.AuthzClusterRole.Aliases,
		Short:   constants.AuthzClusterRole.Short,
		Long:    constants.AuthzClusterRole.Long,
	}

	cmd.AddCommand(
		newListAuthzClusterRoleCmd(),
		newGetAuthzClusterRoleCmd(),
	)

	return cmd
}

func newListAuthzClusterRoleCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListAuthzClusterRole,
		Flags:   []flags.Flag{},
		RunE: func(fg *builder.FlagGetter) error {
			return authzclusterrole.New().List()
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetAuthzClusterRoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetAuthzClusterRole.Use,
		Short:   constants.GetAuthzClusterRole.Short,
		Long:    constants.GetAuthzClusterRole.Long,
		Example: constants.GetAuthzClusterRole.Example,
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return authzclusterrole.New().Get(authzclusterrole.GetParams{
				Name: args[0],
			})
		},
	}

	return cmd
}
