// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterauthzrole

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterauthzrole"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

// NewClusterAuthzRoleCmd returns the parent command for authz cluster role operations
func NewClusterAuthzRoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.ClusterAuthzRole.Use,
		Aliases: constants.ClusterAuthzRole.Aliases,
		Short:   constants.ClusterAuthzRole.Short,
		Long:    constants.ClusterAuthzRole.Long,
	}

	cmd.AddCommand(
		newListClusterAuthzRoleCmd(),
		newGetClusterAuthzRoleCmd(),
		newDeleteClusterAuthzRoleCmd(),
	)

	return cmd
}

func newListClusterAuthzRoleCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListClusterAuthzRole,
		Flags:   []flags.Flag{},
		RunE: func(fg *builder.FlagGetter) error {
			return clusterauthzrole.New().List()
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetClusterAuthzRoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetClusterAuthzRole.Use,
		Short:   constants.GetClusterAuthzRole.Short,
		Long:    constants.GetClusterAuthzRole.Long,
		Example: constants.GetClusterAuthzRole.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clusterauthzrole.New().Get(clusterauthzrole.GetParams{
				Name: args[0],
			})
		},
	}

	return cmd
}

func newDeleteClusterAuthzRoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteClusterAuthzRole.Use,
		Short:   constants.DeleteClusterAuthzRole.Short,
		Long:    constants.DeleteClusterAuthzRole.Long,
		Example: constants.DeleteClusterAuthzRole.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clusterauthzrole.New().Delete(clusterauthzrole.DeleteParams{
				Name: args[0],
			})
		},
	}

	return cmd
}
